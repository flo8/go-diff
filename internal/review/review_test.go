package review

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/semantic"
)

// runGit runs a git command in dir with a fixed identity.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
}

const mainGo = "package main\n\n// Run starts.\nfunc Run() {\n\ttimeout := 500\n\t_ = timeout\n}\n"

// newFixtureRepo builds this history:
//
//	commit 1: main.go, comments.go, old_name.go, gone.go
//	working tree: main.go real change (staged), comments.go comment-only
//	change (unstaged), old_name.go renamed, gone.go deleted, new.go and
//	image.bin untracked.
func newFixtureRepo(t *testing.T) string {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")

	writeFile(t, dir, "main.go", mainGo)
	writeFile(t, dir, "comments.go", "package main\n\n// Old comment.\nvar x = 1\n")
	writeFile(t, dir, "old_name.go", "package main\n\nvar renamed = true\n\nfunc a() {}\nfunc b() {}\n")
	writeFile(t, dir, "gone.go", "package main\n\nvar gone = 1\nvar gone2 = 2\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial")

	writeFile(t, dir, "main.go", "package main\n\n// Run starts now.\nfunc Run() {\n\ttimeout := 1000\n\t_ = timeout\n}\n")
	runGit(t, dir, "add", "main.go")

	writeFile(t, dir, "comments.go", "package main\n\n/* New\n   comment. */\nvar x = 1 // trailing\n")
	runGit(t, dir, "mv", "old_name.go", "new_name.go")
	runGit(t, dir, "rm", "-q", "gone.go")
	writeFile(t, dir, "new.go", "package main\n\n// helper\nfunc helper() int {\n\treturn 1\n}\n")
	writeFile(t, dir, "image.bin", "\x89PNG\x00\x01\x02")

	return dir
}

// loadRange opens the repo, resolves the options and computes all files.
func loadRange(t *testing.T, dir string, opt git.Options) map[string]*File {
	t.Helper()
	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := repo.ResolveRange(opt)
	if err != nil {
		t.Fatal(err)
	}
	files, err := Load(repo, rg)
	if err != nil {
		t.Fatal(err)
	}
	ComputeAll(files, semantic.DefaultFilters)

	byPath := map[string]*File{}
	for _, f := range files {
		byPath[f.Change.Path()] = f
	}
	return byPath
}

// Scenario: working tree vs HEAD with every kind of change.
// Goal: golden counts per file, comment-only file hidden, rename kept.
func TestWorktreeVsHead(t *testing.T) {
	dir := newFixtureRepo(t)
	files := loadRange(t, dir, git.Options{})

	want := map[string]diff.Counts{
		"main.go":     {Modified: 1},
		"new.go":      {Added: 4}, // package, func, return, closing brace
		"gone.go":     {Removed: 3},
		"new_name.go": {},
		"comments.go": {},
	}
	for path, counts := range want {
		f, ok := files[path]
		if !ok {
			t.Errorf("%s: not listed", path)
			continue
		}
		if f.Counts != counts {
			t.Errorf("%s: counts = %+v, want %+v", path, f.Counts, counts)
		}
	}

	// Comment-only change is listed by git but is not meaningful.
	if files["comments.go"].Meaningful() {
		t.Errorf("comments.go should not be meaningful")
	}

	// A pure rename stays visible.
	renamed := files["new_name.go"]
	if renamed.Change.Status != git.Renamed || renamed.Change.OldPath != "old_name.go" || !renamed.Meaningful() {
		t.Errorf("rename not detected: %+v", renamed.Change)
	}

	// Binary files are flagged and not compared.
	if bin := files["image.bin"]; bin == nil || !bin.Binary || !bin.Meaningful() {
		t.Errorf("image.bin should be a meaningful binary file: %+v", bin)
	}
}

// Scenario: staged vs HEAD.
// Goal: only the staged edits show up (main.go, the rename, the deletion),
// not the unstaged comment change nor untracked files.
func TestStagedVsHead(t *testing.T) {
	dir := newFixtureRepo(t)
	files := loadRange(t, dir, git.Options{Staged: true})

	for _, path := range []string{"main.go", "new_name.go", "gone.go"} {
		if files[path] == nil {
			t.Errorf("%s: missing from staged changes", path)
		}
	}
	for _, path := range []string{"comments.go", "new.go", "image.bin"} {
		if files[path] != nil {
			t.Errorf("%s: should not be in staged changes", path)
		}
	}
}

// Scenario: unstaged changes (working tree vs index).
// Goal: the staged main.go edit is not shown, the unstaged and untracked are.
func TestUnstaged(t *testing.T) {
	dir := newFixtureRepo(t)
	files := loadRange(t, dir, git.Options{Unstaged: true})

	if files["main.go"] != nil {
		t.Errorf("main.go is fully staged and should not appear")
	}
	for _, path := range []string{"comments.go", "new.go"} {
		if files[path] == nil {
			t.Errorf("%s: missing from unstaged changes", path)
		}
	}
}

// Scenario: a commit compared with its parent, and the root commit.
// Goal: the commit shows its own change; the root commit shows all files added.
func TestCommitVsParent(t *testing.T) {
	dir := newFixtureRepo(t)
	runGit(t, dir, "commit", "-q", "-m", "second")

	files := loadRange(t, dir, git.Options{Commit: "HEAD"})
	if f := files["main.go"]; f == nil || f.Counts != (diff.Counts{Modified: 1}) {
		t.Errorf("HEAD main.go = %+v", f)
	}

	root := loadRange(t, dir, git.Options{Commit: "HEAD~1"})
	if f := root["main.go"]; f == nil || f.Change.Status != git.Added || f.Counts.Added != 5 {
		t.Errorf("root main.go = %+v", f)
	}
}

// Scenario: branch comparison with two dots and three dots.
// Goal: "main...feature" shows only what the feature branch changed.
func TestBranchComparison(t *testing.T) {
	dir := newFixtureRepo(t)
	runGit(t, dir, "stash", "-q", "--include-untracked")
	runGit(t, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, dir, "feature.go", "package main\n\nvar feature = 1\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "feature")
	runGit(t, dir, "checkout", "-q", "main")
	writeFile(t, dir, "mainonly.go", "package main\n\nvar m = 1\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "main work")

	threeDot := loadRange(t, dir, git.Options{Args: []string{"main...feature"}})
	if threeDot["feature.go"] == nil || threeDot["mainonly.go"] != nil || len(threeDot) != 1 {
		t.Errorf("main...feature = %v", keys(threeDot))
	}

	twoArgs := loadRange(t, dir, git.Options{Args: []string{"main", "feature"}})
	if twoArgs["feature.go"] == nil || twoArgs["mainonly.go"] == nil {
		t.Errorf("main feature = %v", keys(twoArgs))
	}
}

func keys(m map[string]*File) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Scenario: run every mode against a repository with staged and unstaged work.
// Goal: the index file and the working tree are byte-for-byte unchanged.
func TestNeverModifiesIndexOrWorktree(t *testing.T) {
	dir := newFixtureRepo(t)
	indexBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	mainBefore, _ := os.ReadFile(filepath.Join(dir, "main.go"))

	loadRange(t, dir, git.Options{})
	loadRange(t, dir, git.Options{Staged: true})
	loadRange(t, dir, git.Options{Unstaged: true})

	indexAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	mainAfter, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Errorf(".git/index was modified")
	}
	if !bytes.Equal(mainBefore, mainAfter) {
		t.Errorf("main.go was modified")
	}
}
