package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testEnv gives git a fixed identity and ignores the user's config.
var testEnv = []string{
	"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
}

// gitIn runs a git command in dir and returns its output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), testEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeFile writes content to dir/name.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
}

// newRepo creates a repository with one commit of a.txt. The process
// environment is set to testEnv so that the Repo methods, which inherit
// it, commit with the fixed identity.
func newRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	for _, kv := range testEnv {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}

	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "a.txt", "one\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "first")

	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo, dir
}

// withRemote adds a bare repository as origin, without pushing to it.
func withRemote(t *testing.T, dir string) string {
	t.Helper()
	bare := t.TempDir()
	gitIn(t, bare, "init", "-q", "--bare", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", bare)
	return bare
}

// mustStatus reads the status or fails the test.
func mustStatus(t *testing.T, repo *Repo) WorkStatus {
	t.Helper()
	st, err := repo.Status()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// Scenario: one staged file, one partly staged, one unstaged, one
// untracked in a subfolder, and a staged rename.
// Goal: Status reports each file's stage under its path, and the branch.
func TestStatusStages(t *testing.T) {
	repo, dir := newRepo(t)
	writeFile(t, dir, "b.txt", "b\n")
	writeFile(t, dir, "c.txt", "c\n")
	writeFile(t, dir, "d.txt", "d\n")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "more")

	writeFile(t, dir, "a.txt", "two\n")
	gitIn(t, dir, "add", "a.txt")
	writeFile(t, dir, "b.txt", "b2\n")
	gitIn(t, dir, "add", "b.txt")
	writeFile(t, dir, "b.txt", "b3\n")
	writeFile(t, dir, "c.txt", "c2\n")
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	writeFile(t, dir, "sub/new.txt", "new\n")
	gitIn(t, dir, "mv", "d.txt", "e.txt")

	st := mustStatus(t, repo)
	want := map[string]Stage{
		"a.txt": Staged, "b.txt": Partly, "c.txt": Unstaged,
		"sub/new.txt": Unstaged, "e.txt": Staged,
	}
	for path, stage := range want {
		if st.Files[path] != stage {
			t.Errorf("%s: stage %d, want %d", path, st.Files[path], stage)
		}
	}
	if len(st.Files) != len(want) {
		t.Errorf("files = %v, want %d entries", st.Files, len(want))
	}
	if st.Branch != "main" || st.Upstream != "" {
		t.Errorf("branch %q upstream %q", st.Branch, st.Upstream)
	}
}

// Scenario: stage and unstage one file, then everything.
// Goal: the index follows, and the file on disk never changes.
func TestStageUnstage(t *testing.T) {
	repo, dir := newRepo(t)
	writeFile(t, dir, "a.txt", "two\n")
	writeFile(t, dir, "new.txt", "new\n")

	if err := repo.Stage([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.Files["a.txt"] != Staged || st.Files["new.txt"] != Unstaged {
		t.Errorf("after Stage: %v", st.Files)
	}

	if err := repo.Unstage([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.HasStaged() {
		t.Errorf("after Unstage: %v", st.Files)
	}

	if err := repo.StageAll(); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.HasUnstaged() {
		t.Errorf("after StageAll: %v", st.Files)
	}

	if err := repo.UnstageAll(); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.HasStaged() {
		t.Errorf("after UnstageAll: %v", st.Files)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "two\n" {
		t.Errorf("a.txt changed on disk: %q", data)
	}
}

// Scenario: stage a deleted file, then unstage a staged rename by both
// of its paths.
// Goal: deletions stage like any change; unstaging the rename leaves
// nothing staged.
func TestStageDeleteAndRename(t *testing.T) {
	repo, dir := newRepo(t)
	os.Remove(filepath.Join(dir, "a.txt"))

	if err := repo.Stage([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.Files["a.txt"] != Staged {
		t.Errorf("deletion not staged: %v", st.Files)
	}
	gitIn(t, dir, "reset", "-q")
	gitIn(t, dir, "checkout", "--", "a.txt")

	gitIn(t, dir, "mv", "a.txt", "b.txt")
	if err := repo.Unstage([]string{"a.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.HasStaged() {
		t.Errorf("rename still staged: %v", st.Files)
	}
}

// Scenario: a repository without any commit.
// Goal: stage and unstage still work (there is no HEAD to restore from).
func TestUnstageBeforeFirstCommit(t *testing.T) {
	for _, kv := range testEnv {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	writeFile(t, dir, "a.txt", "one\n")
	repo, _ := Open(dir)

	if err := repo.StageAll(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Unstage([]string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.StageAll(); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnstageAll(); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, repo); st.HasStaged() || st.Files["a.txt"] != Unstaged {
		t.Errorf("status = %v", st.Files)
	}
}

// Scenario: commit a staged change, then a failing pre-commit hook.
// Goal: the commit's first output line names it; the hook's output is
// the error.
func TestCommit(t *testing.T) {
	repo, dir := newRepo(t)
	writeFile(t, dir, "a.txt", "two\n")
	repo.StageAll()

	first, err := repo.Commit("second", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "[main ") || !strings.HasSuffix(first, "] second") {
		t.Errorf("first line = %q", first)
	}

	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	os.WriteFile(hook, []byte("#!/bin/sh\necho lint failed\nexit 1\n"), 0o755)
	writeFile(t, dir, "a.txt", "three\n")
	repo.StageAll()
	_, err = repo.Commit("third", nil)
	if err == nil || !strings.Contains(err.Error(), "lint failed") {
		t.Errorf("err = %v, want the hook output", err)
	}
}

// Scenario: push a branch that has no upstream, then commit and push
// again, with progress lines collected.
// Goal: the first push sets origin/main as upstream; the second updates
// it; ahead goes back to 0.
func TestPush(t *testing.T) {
	repo, dir := newRepo(t)
	bare := withRemote(t, dir)

	if _, err := repo.Push(mustStatus(t, repo), nil); err != nil {
		t.Fatal(err)
	}
	st := mustStatus(t, repo)
	if st.Upstream != "origin/main" || st.Ahead != 0 {
		t.Fatalf("after first push: upstream %q ahead %d", st.Upstream, st.Ahead)
	}

	writeFile(t, dir, "a.txt", "two\n")
	repo.StageAll()
	repo.Commit("second", nil)
	if st = mustStatus(t, repo); st.Ahead != 1 {
		t.Errorf("ahead = %d, want 1", st.Ahead)
	}

	lines := make(chan string, 100)
	if _, err := repo.Push(st, lines); err != nil {
		t.Fatal(err)
	}
	if gitIn(t, bare, "log", "-1", "--format=%s", "main") != "second" {
		t.Errorf("remote main is not at the new commit")
	}
	if len(lines) == 0 {
		t.Errorf("no output lines were sent")
	}
}

// Scenario: a second clone pushes a commit; then both sides commit.
// Goal: the first pull fast-forwards; once the branches have diverged,
// pull fails instead of merging, and the work tree is untouched.
func TestPullFastForwardOnly(t *testing.T) {
	repo, dir := newRepo(t)
	bare := withRemote(t, dir)
	repo.Push(mustStatus(t, repo), nil)

	other := t.TempDir()
	gitIn(t, other, "clone", "-q", bare, ".")
	writeFile(t, other, "b.txt", "b\n")
	gitIn(t, other, "add", ".")
	gitIn(t, other, "commit", "-q", "-m", "from other")
	gitIn(t, other, "push", "-q")

	if _, err := repo.Pull(mustStatus(t, repo), nil); err != nil {
		t.Fatal(err)
	}
	if gitIn(t, dir, "log", "-1", "--format=%s") != "from other" {
		t.Errorf("pull did not fast-forward")
	}

	writeFile(t, other, "b.txt", "b2\n")
	gitIn(t, other, "commit", "-q", "-am", "other again")
	gitIn(t, other, "push", "-q")
	writeFile(t, dir, "a.txt", "mine\n")
	repo.StageAll()
	repo.Commit("mine", nil)

	if _, err := repo.Pull(mustStatus(t, repo), nil); err == nil {
		t.Errorf("pull of diverged branches should fail")
	}
	if gitIn(t, dir, "log", "-1", "--format=%s") != "mine" {
		t.Errorf("a failed pull should not move the branch")
	}
}

// Scenario: pull without upstream, push with a detached HEAD.
// Goal: both fail with a message before running git.
func TestPullPushPreconditions(t *testing.T) {
	repo, dir := newRepo(t)

	if _, err := repo.Pull(mustStatus(t, repo), nil); err == nil || !strings.Contains(err.Error(), "no upstream") {
		t.Errorf("pull err = %v", err)
	}

	gitIn(t, dir, "checkout", "-q", "--detach")
	if _, err := repo.Push(mustStatus(t, repo), nil); err == nil || !strings.Contains(err.Error(), "detached") {
		t.Errorf("push err = %v", err)
	}
}
