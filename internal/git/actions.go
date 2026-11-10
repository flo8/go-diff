package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// This file holds every git command that changes the repository: stage,
// unstage, commit, pull and push. They run only when the user presses
// their key. Everything else in the package is read-only.

// outputTail is how many lines of a failed command's output are kept
// for the error message.
const outputTail = 10

// Stage adds the changes of paths to the index, deletions included.
func (r *Repo) Stage(paths []string) error {
	var args []string

	args = append([]string{"add", "-A", "--"}, paths...)
	_, err := r.write(nil, args...)
	return err
}

// Unstage takes paths out of the index, keeping the files as they are.
// Before the first commit there is no HEAD to restore from, so the files
// are removed from the index instead.
func (r *Repo) Unstage(paths []string) error {
	var args []string

	args = append([]string{"restore", "--staged", "--"}, paths...)
	if !r.hasHead() {
		args = append([]string{"rm", "--cached", "-r", "-q", "--"}, paths...)
	}

	_, err := r.write(nil, args...)
	return err
}

// StageAll stages every change, untracked files included.
func (r *Repo) StageAll() error {
	_, err := r.write(nil, "add", "-A")
	return err
}

// UnstageAll empties the staged changes, keeping the files as they are.
func (r *Repo) UnstageAll() error {
	var err error

	if !r.hasHead() {
		_, err = r.write(nil, "rm", "--cached", "-r", "-q", ".")
		return err
	}

	_, err = r.write(nil, "reset", "-q")
	return err
}

// Commit commits the staged changes with a message. Hooks run as usual.
// It returns git's first output line, like "[main 1a2b3c4] Fix typo".
func (r *Repo) Commit(message string, lines chan<- string) (string, error) {
	var out string
	var err error

	out, err = r.write(lines, "commit", "-m", message)
	if err != nil {
		return "", err
	}

	first, _, _ := strings.Cut(out, "\n")
	return first, nil
}

// CommitCmd returns "git commit" for the user's editor: it needs the
// terminal, so the caller runs it and hands over the screen.
func (r *Repo) CommitCmd() *exec.Cmd {
	var cmd *exec.Cmd

	cmd = exec.Command("git", "commit")
	cmd.Dir = r.Root
	return cmd
}

// Pull fast-forwards the branch to its upstream. It never merges or
// rebases: when the branches have diverged it fails and says so.
func (r *Repo) Pull(st WorkStatus, lines chan<- string) (string, error) {
	if st.Upstream == "" {
		return "", errors.New("no upstream branch to pull from (push first)")
	}
	return r.write(lines, "pull", "--no-rebase", "--ff-only", "--progress")
}

// Push sends the branch to its upstream. A branch without one is pushed
// to origin (or the only remote) and that becomes its upstream. It never
// forces.
func (r *Repo) Push(st WorkStatus, lines chan<- string) (string, error) {
	var remote string
	var err error

	if st.Branch == "" {
		return "", errors.New("HEAD is detached: no branch to push")
	}
	if st.Upstream != "" {
		return r.write(lines, "push", "--progress")
	}

	remote, err = r.PushRemote()
	if err != nil {
		return "", err
	}
	return r.write(lines, "push", "--progress", "-u", remote, st.Branch)
}

// PushRemote is the remote a branch without upstream is pushed to:
// origin, or the only remote.
func (r *Repo) PushRemote() (string, error) {
	var out []byte
	var remotes []string
	var err error

	out, err = r.run("remote")
	if err != nil {
		return "", err
	}

	remotes = strings.Fields(string(out))
	for _, name := range remotes {
		if name == "origin" {
			return name, nil
		}
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	return "", fmt.Errorf("no upstream and %d remotes: push once from the shell to choose", len(remotes))
}

// hasHead reports whether the repository has a commit yet.
func (r *Repo) hasHead() bool {
	_, err := r.ResolveCommit("HEAD")
	return err == nil
}

// write runs a git command that changes the repository. Each line it
// prints is sent to lines, if not nil; progress lines end with \r rather
// than \n and are sent too. It returns the output, or an error with the
// last lines of the output.
//
// Git must never wait for input that the TUI cannot give, so password
// prompts are turned off: the command fails instead.
func (r *Repo) write(lines chan<- string, args ...string) (string, error) {
	var cmd *exec.Cmd
	var reader *os.File
	var writer *os.File
	var out []string
	var err error

	cmd = exec.Command("git", args...)
	cmd.Dir = r.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, r.sshEnv()...)

	// stdout and stderr share one pipe so lines arrive in order.
	reader, writer, err = os.Pipe()
	if err != nil {
		return "", err
	}
	cmd.Stdout = writer
	cmd.Stderr = writer

	err = cmd.Start()
	writer.Close()
	if err != nil {
		reader.Close()
		return "", err
	}

	out = readLines(reader, lines)
	reader.Close()
	err = cmd.Wait()

	return commandResult(out, err)
}

// readLines reads a command's output until it ends, sending each line
// to lines (when not nil) and keeping the lines that are not progress.
func readLines(reader *os.File, lines chan<- string) []string {
	var scanner *bufio.Scanner
	var out []string

	scanner = bufio.NewScanner(reader)
	scanner.Split(scanLinesOrCR)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if lines != nil {
			lines <- line
		}
		if !strings.Contains(line, "%") {
			out = append(out, line)
		}
	}

	return out
}

// commandResult turns the output and exit error of a command into its
// result: the output, or an error made of its last lines.
func commandResult(out []string, err error) (string, error) {
	var tail []string

	if err == nil {
		return strings.Join(out, "\n"), nil
	}

	tail = out[max(len(out)-outputTail, 0):]
	if len(tail) == 0 {
		return "", err
	}
	return "", errors.New(strings.Join(tail, "\n"))
}

// scanLinesOrCR splits output on \n and on \r, which git uses to redraw
// a progress line in place.
func scanLinesOrCR(data []byte, atEOF bool) (int, []byte, error) {
	var i int

	i = bytes.IndexAny(data, "\r\n")
	if i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// sshEnv keeps ssh from asking for a passphrase on the terminal, which
// the TUI owns: keys from the ssh agent work, others fail. A user's own
// ssh command is left alone.
func (r *Repo) sshEnv() []string {
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return nil
	}
	if _, err := r.run("config", "core.sshCommand"); err == nil {
		return nil
	}
	return []string{"GIT_SSH_COMMAND=ssh -o BatchMode=yes"}
}
