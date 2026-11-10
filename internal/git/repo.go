// Package git reads changed files and file contents from a Git repository.
//
// It only runs read-only Git commands. It never writes to the working tree
// or the index: every command runs with --no-optional-locks so that Git
// does not even refresh the index stat cache as a side effect.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// EmptyTree is the hash of the empty tree in SHA-1 repositories.
// It is used as the base when a commit has no parent or HEAD does not exist.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Repo is a Git repository on disk.
type Repo struct {
	Root string // absolute path of the top-level directory
}

// Open finds the repository that contains dir.
func Open(dir string) (*Repo, error) {
	var repo *Repo
	var out []byte
	var err error

	repo = &Repo{Root: dir}
	out, err = repo.run("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}

	repo.Root = strings.TrimSpace(string(out))
	return repo, nil
}

// run executes a read-only git command in the repository root
// and returns its standard output.
func (r *Repo) run(args ...string) ([]byte, error) {
	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var full []string
	var err error

	// Never take optional locks: this keeps git from rewriting the index.
	full = append([]string{"--no-optional-locks", "-c", "core.quotepath=off"}, args...)
	cmd = exec.Command("git", full...)
	cmd.Dir = r.Root
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, err
		}
		return nil, errors.New(msg)
	}

	return stdout.Bytes(), nil
}

// ResolveCommit returns the full hash of rev, or an error if rev
// does not name a commit.
func (r *Repo) ResolveCommit(rev string) (string, error) {
	var out []byte
	var err error

	out, err = r.run("rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("unknown revision %q", rev)
	}

	return strings.TrimSpace(string(out)), nil
}

// MergeBase returns the best common ancestor of a and b.
func (r *Repo) MergeBase(a, b string) (string, error) {
	var out []byte
	var err error

	out, err = r.run("merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("no merge base between %q and %q: %w", a, b, err)
	}

	return strings.TrimSpace(string(out)), nil
}
