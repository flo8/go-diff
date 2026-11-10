package git

import (
	"fmt"
	"strconv"
	"strings"
)

// Commit is one entry of the history.
type Commit struct {
	Hash    string
	Short   string
	Author  string
	Date    string // relative, like "3 days ago"
	Subject string
}

// Field and record separators for "git log --format": they cannot appear
// in names or subjects.
const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

// Log returns the latest commits reachable from HEAD, newest first.
// A repository without commits has an empty history.
func (r *Repo) Log(limit int) ([]Commit, error) {
	var format string
	var out []byte
	var err error

	_, err = r.ResolveCommit("HEAD")
	if err != nil {
		return nil, nil
	}

	format = "--format=" + strings.Join([]string{"%H", "%h", "%an", "%ar", "%s"}, "%x1f") + "%x1e"
	out, err = r.run("log", "-n", strconv.Itoa(limit), format)
	if err != nil {
		return nil, err
	}

	return parseLog(string(out))
}

// parseLog reads the records written by Log's format.
func parseLog(out string) ([]Commit, error) {
	var commits []Commit

	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, fieldSep)
		if len(fields) != 5 {
			return nil, fmt.Errorf("unexpected git log record %q", record)
		}

		commits = append(commits, Commit{
			Hash:    fields[0],
			Short:   fields[1],
			Author:  fields[2],
			Date:    fields[3],
			Subject: fields[4],
		})
	}

	return commits, nil
}

// Unpushed returns the hashes of the commits on HEAD that are not pushed:
// not on the upstream, or, without one, not on any remote branch.
// Without commits the set is empty.
func (r *Repo) Unpushed(upstream string) (map[string]bool, error) {
	var args []string
	var out []byte
	var hashes map[string]bool
	var err error

	hashes = map[string]bool{}
	if !r.hasHead() {
		return hashes, nil
	}

	// Without upstream, compare with every remote branch.
	args = []string{"rev-list", "HEAD", "--not", "--remotes"}
	if upstream != "" {
		args = []string{"rev-list", "HEAD", "--not", upstream}
	}

	out, err = r.run(args...)
	if err != nil {
		return nil, err
	}

	for _, hash := range strings.Fields(string(out)) {
		hashes[hash] = true
	}
	return hashes, nil
}
