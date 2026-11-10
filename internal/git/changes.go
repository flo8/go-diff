package git

import (
	"bytes"
	"fmt"
)

// Status is the kind of change git reports for a file.
type Status byte

const (
	Added     Status = 'A'
	Deleted   Status = 'D'
	Modified  Status = 'M'
	Renamed   Status = 'R'
	Copied    Status = 'C'
	TypeChg   Status = 'T'
	Untracked Status = '?'
)

// Change is one changed file between the two sides of a Range.
type Change struct {
	Status  Status
	OldPath string // empty for added and untracked files
	NewPath string // empty for deleted files
}

// Path returns the path to show for the file: the new path,
// or the old one for deleted files.
func (c Change) Path() string {
	if c.NewPath != "" {
		return c.NewPath
	}
	return c.OldPath
}

// Changes lists the files that differ between the two sides of rg.
// When the target is the working tree, untracked files are included as
// new files (files ignored by .gitignore are not).
func (r *Repo) Changes(rg Range) ([]Change, error) {
	var args []string
	var out []byte
	var changes []Change
	var untracked []Change
	var err error

	args, err = diffArgs(rg)
	if err != nil {
		return nil, err
	}

	out, err = r.run(args...)
	if err != nil {
		return nil, err
	}

	changes, err = parseNameStatus(out)
	if err != nil {
		return nil, err
	}

	// git diff does not report untracked files, so list them separately.
	if rg.Target.Kind == FromWorktree {
		untracked, err = r.untracked()
		if err != nil {
			return nil, err
		}
		changes = append(changes, untracked...)
	}

	return changes, nil
}

// diffArgs builds the "git diff --name-status" command for a range.
func diffArgs(rg Range) ([]string, error) {
	var args []string

	args = []string{"diff", "--name-status", "-z", "-M", "--ignore-submodules", "--no-ext-diff"}
	base := rg.Base
	target := rg.Target

	switch {
	case base.Kind == FromRev && target.Kind == FromRev:
		args = append(args, base.Rev, target.Rev)
	case base.Kind == FromRev && target.Kind == FromIndex:
		args = append(args, "--cached", base.Rev)
	case base.Kind == FromRev && target.Kind == FromWorktree:
		args = append(args, base.Rev)
	case base.Kind == FromIndex && target.Kind == FromWorktree:
		// plain "git diff" compares the index with the working tree
	default:
		return nil, fmt.Errorf("unsupported comparison %s", rg)
	}

	args = append(args, "--")
	return args, nil
}

// parseNameStatus parses the NUL-separated output of
// "git diff --name-status -z". Each record is a status field followed by
// one path, or two paths for renames and copies ("R087\0old\0new\0").
func parseNameStatus(out []byte) ([]Change, error) {
	var fields [][]byte
	var changes []Change
	var i int

	fields = bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	if len(out) == 0 {
		return nil, nil
	}

	for i < len(fields) {
		status := Status(fields[i][0])
		i++

		// Renames and copies carry the old and the new path.
		if status == Renamed || status == Copied {
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("truncated rename record in git output")
			}
			changes = append(changes, Change{Status: status, OldPath: string(fields[i]), NewPath: string(fields[i+1])})
			i += 2
			continue
		}

		if i >= len(fields) {
			return nil, fmt.Errorf("truncated record in git output")
		}
		changes = append(changes, newChange(status, string(fields[i])))
		i++
	}

	return changes, nil
}

// newChange fills in the old and new paths for a single-path record.
func newChange(status Status, path string) Change {
	switch status {
	case Added:
		return Change{Status: status, NewPath: path}
	case Deleted:
		return Change{Status: status, OldPath: path}
	}
	return Change{Status: status, OldPath: path, NewPath: path}
}

// untracked lists files in the working tree that git does not track
// and that are not ignored.
func (r *Repo) untracked() ([]Change, error) {
	var out []byte
	var changes []Change
	var err error

	out, err = r.run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}

	for _, path := range bytes.Split(out, []byte{0}) {
		if len(path) == 0 {
			continue
		}
		changes = append(changes, Change{Status: Untracked, NewPath: string(path)})
	}

	return changes, nil
}
