package git

import (
	"fmt"
	"strings"
)

// SideKind says where one side of a comparison reads files from.
type SideKind int

const (
	FromRev      SideKind = iota // a commit, branch or tree
	FromIndex                    // the staging area
	FromWorktree                 // files on disk
)

// Side is one end of a comparison.
type Side struct {
	Kind SideKind
	Rev  string // resolved hash, only for FromRev
	Name string // what the user typed, for display
}

// Range is the pair of sides being compared: Base is shown on the left
// ("before") and Target on the right ("after").
type Range struct {
	Base   Side
	Target Side
}

// Options are the command-line choices that select a Range.
type Options struct {
	Staged   bool     // index vs HEAD
	Unstaged bool     // working tree vs index
	Commit   string   // commit vs its parent
	Args     []string // [base] or [base target], or "a..b", "a...b"
}

// String describes the range for the header line.
func (rg Range) String() string {
	return rg.Base.Name + " → " + rg.Target.Name
}

// ResolveRange turns command-line options into a Range.
//
//	(no options)      working tree vs HEAD
//	Staged            index vs HEAD
//	Unstaged          working tree vs index
//	Commit=REV        REV^ vs REV
//	Args=[BASE]       working tree vs BASE
//	Args=[BASE TGT]   BASE vs TGT
//	Args=[A..B]       A vs B
//	Args=[A...B]      merge-base(A, B) vs B (what a pull request shows)
func (r *Repo) ResolveRange(opt Options) (Range, error) {
	var worktree Side
	var index Side

	worktree = Side{Kind: FromWorktree, Name: "working tree"}
	index = Side{Kind: FromIndex, Name: "index"}

	switch {
	case opt.Staged:
		return Range{Base: r.headOrEmpty(), Target: index}, nil
	case opt.Unstaged:
		return Range{Base: index, Target: worktree}, nil
	case opt.Commit != "":
		return r.commitRange(opt.Commit)
	case len(opt.Args) == 0:
		return Range{Base: r.headOrEmpty(), Target: worktree}, nil
	case len(opt.Args) == 1:
		return r.singleArgRange(opt.Args[0])
	case len(opt.Args) == 2:
		return r.revPair(opt.Args[0], opt.Args[1])
	}

	return Range{}, fmt.Errorf("expected at most two revisions, got %d", len(opt.Args))
}

// singleArgRange handles "A...B", "A..B" and a plain "BASE".
func (r *Repo) singleArgRange(arg string) (Range, error) {
	var a, b string
	var found bool
	var base string
	var err error

	// Three dots: compare B against the point where it forked from A.
	a, b, found = strings.Cut(arg, "...")
	if found {
		base, err = r.MergeBase(orHead(a), orHead(b))
		if err != nil {
			return Range{}, err
		}
		return r.revPair(base, orHead(b))
	}

	// Two dots: plain A vs B.
	a, b, found = strings.Cut(arg, "..")
	if found {
		return r.revPair(orHead(a), orHead(b))
	}

	// A single revision: compare the working tree against it.
	base, err = r.ResolveCommit(arg)
	if err != nil {
		return Range{}, err
	}
	return Range{
		Base:   Side{Kind: FromRev, Rev: base, Name: arg},
		Target: Side{Kind: FromWorktree, Name: "working tree"},
	}, nil
}

// revPair compares two revisions.
func (r *Repo) revPair(base, target string) (Range, error) {
	var baseHash, targetHash string
	var err error

	baseHash, err = r.ResolveCommit(base)
	if err != nil {
		return Range{}, err
	}

	targetHash, err = r.ResolveCommit(target)
	if err != nil {
		return Range{}, err
	}

	return Range{
		Base:   Side{Kind: FromRev, Rev: baseHash, Name: shortName(base)},
		Target: Side{Kind: FromRev, Rev: targetHash, Name: shortName(target)},
	}, nil
}

// commitRange compares a commit with its first parent. A root commit
// is compared with the empty tree.
func (r *Repo) commitRange(rev string) (Range, error) {
	var hash string
	var parent string
	var err error

	hash, err = r.ResolveCommit(rev)
	if err != nil {
		return Range{}, err
	}

	parent, err = r.ResolveCommit(hash + "^")
	if err != nil {
		parent = EmptyTree
	}

	return Range{
		Base:   Side{Kind: FromRev, Rev: parent, Name: shortName(rev) + "^"},
		Target: Side{Kind: FromRev, Rev: hash, Name: shortName(rev)},
	}, nil
}

// headOrEmpty returns HEAD, or the empty tree in a repository
// without commits yet.
func (r *Repo) headOrEmpty() Side {
	var hash string
	var err error

	hash, err = r.ResolveCommit("HEAD")
	if err != nil {
		return Side{Kind: FromRev, Rev: EmptyTree, Name: "empty tree"}
	}
	return Side{Kind: FromRev, Rev: hash, Name: "HEAD"}
}

// orHead treats an empty side of "a..b" as HEAD, like git does.
func orHead(rev string) string {
	if rev == "" {
		return "HEAD"
	}
	return rev
}

// shortName keeps user-typed names but shortens full hashes.
func shortName(rev string) string {
	if len(rev) == 40 || len(rev) == 64 {
		return rev[:8]
	}
	return rev
}
