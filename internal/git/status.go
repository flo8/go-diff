package git

import (
	"bytes"
	"strconv"
	"strings"
)

// Stage says how much of a file's change is in the index.
type Stage int

const (
	Unstaged Stage = iota // nothing staged
	Partly                // some staged, some not
	Staged                // everything staged
)

// WorkStatus is the branch and the staging state of each changed file.
type WorkStatus struct {
	Branch   string           // "" when HEAD is detached
	Upstream string           // "origin/main", or "" when there is none
	Ahead    int              // commits on the branch, not on the upstream
	Behind   int              // commits on the upstream, not on the branch
	Files    map[string]Stage // by path; files without changes are absent
}

// Status reads the branch and the staging state of every changed file
// with one "git status" call. It does not change the index.
func (r *Repo) Status() (WorkStatus, error) {
	var out []byte
	var records [][]byte
	var st WorkStatus
	var err error

	out, err = r.run("status", "--porcelain=v2", "--branch", "--untracked-files=all", "-z")
	if err != nil {
		return st, err
	}

	st.Files = map[string]Stage{}
	records = bytes.Split(out, []byte{0})
	for i := 0; i < len(records); i++ {
		line := string(records[i])

		// A rename record is followed by its old path, which is skipped.
		if strings.HasPrefix(line, "2 ") {
			i++
		}
		parseStatusLine(line, &st)
	}

	return st, nil
}

// parseStatusLine reads one record of "git status --porcelain=v2":
//
//	# branch.head main
//	# branch.upstream origin/main
//	# branch.ab +1 -2
//	1 XY sub mH mI mW hH hI path
//	2 XY sub mH mI mW hH hI score path
//	u XY sub m1 m2 m3 mW h1 h2 h3 path
//	? path
//
// X is the staged change, Y the unstaged one, "." meaning none.
func parseStatusLine(line string, st *WorkStatus) {
	var fields []string

	switch {
	case strings.HasPrefix(line, "# branch.head "):
		st.Branch = strings.TrimPrefix(line, "# branch.head ")
		if st.Branch == "(detached)" {
			st.Branch = ""
		}
	case strings.HasPrefix(line, "# branch.upstream "):
		st.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
	case strings.HasPrefix(line, "# branch.ab "):
		fields = strings.Fields(line)
		st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
		st.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
	case strings.HasPrefix(line, "1 "):
		fields = strings.SplitN(line, " ", 9)
		st.Files[fields[8]] = stageOf(fields[1])
	case strings.HasPrefix(line, "2 "):
		fields = strings.SplitN(line, " ", 10)
		st.Files[fields[9]] = stageOf(fields[1])
	case strings.HasPrefix(line, "u "):
		fields = strings.SplitN(line, " ", 11)
		st.Files[fields[10]] = Unstaged
	case strings.HasPrefix(line, "? "):
		st.Files[strings.TrimPrefix(line, "? ")] = Unstaged
	}
}

// stageOf turns the XY letters into a Stage.
func stageOf(xy string) Stage {
	var staged bool
	var unstaged bool

	staged = xy[0] != '.'
	unstaged = xy[1] != '.'

	switch {
	case staged && unstaged:
		return Partly
	case staged:
		return Staged
	}
	return Unstaged
}

// HasStaged reports whether any file has a staged change.
func (st WorkStatus) HasStaged() bool {
	for _, s := range st.Files {
		if s != Unstaged {
			return true
		}
	}
	return false
}

// HasUnstaged reports whether any file has an unstaged change.
func (st WorkStatus) HasUnstaged() bool {
	for _, s := range st.Files {
		if s != Staged {
			return true
		}
	}
	return false
}
