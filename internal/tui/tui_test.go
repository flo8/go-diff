package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/theme"
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

// commitFile writes one file and commits it.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", message)
}

// newTestModel builds this history and a model on it, sized 120x30:
//
//	"add a"      a.go created
//	"change a"   a.go: 1 line modified
//	"add b"      b.go created
//	working tree: b.go has one unstaged modified line
func newTestModel(t *testing.T) *Model {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	commitFile(t, dir, "a.go", "package a\n\nvar x = 1\n", "add a")
	commitFile(t, dir, "a.go", "package a\n\nvar x = 2\n", "change a")
	commitFile(t, dir, "b.go", "package a\n\nvar y = 1\n", "add b")
	err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package a\n\nvar y = 3\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := repo.ResolveRange(git.Options{})
	if err != nil {
		t.Fatal(err)
	}
	files, err := review.Load(repo, rg)
	if err != nil {
		t.Fatal(err)
	}
	review.ComputeAll(files, semantic.DefaultFilters)

	m := New(Options{
		Repo: repo, Files: files, Range: rg.String(),
		Filters: semantic.DefaultFilters, Themes: theme.Builtins(), Theme: "dark",
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m
}

// press sends a key and runs the commands it returns, and the commands
// those return, the way the Bubble Tea loop would.
func press(m *Model, key string) {
	run(m, m.handleKey(key))
}

func run(m *Model, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		_, cmd = m.Update(msg)
	}
}

// Scenario: open the log tab and move through the history.
// Goal: commits are listed newest first, and each commit shows its own
// files with the right counts once loaded.
func TestLogShowsEachCommitsFiles(t *testing.T) {
	m := newTestModel(t)

	press(m, "2")
	if m.tab != tabLog || len(m.log.commits) != 3 {
		t.Fatalf("tab=%d commits=%d, want log tab with 3 commits", m.tab, len(m.log.commits))
	}
	if m.log.commits[0].Subject != "add b" {
		t.Errorf("first commit = %q, want newest (add b)", m.log.commits[0].Subject)
	}

	// Newest commit: b.go added (2 code lines).
	set := m.log.files.set
	if set == nil || len(set.list.visible) != 1 || set.list.visible[0].Change.Path() != "b.go" {
		t.Fatalf("add b files = %+v", set)
	}
	if got := set.list.visible[0].Counts.Added; got != 2 {
		t.Errorf("add b: added = %d, want 2", got)
	}

	// One commit down: a.go with one modified line.
	press(m, "down")
	set = m.log.files.set
	if set == nil || set.list.visible[0].Change.Path() != "a.go" || set.list.visible[0].Counts.Modified != 1 {
		t.Errorf("change a files = %+v", set)
	}

	// The view draws without panicking and names the commit.
	if !strings.Contains(m.logView(), "change a") {
		t.Errorf("log view does not show the selected commit")
	}
}

// Scenario: open a file in the log tab, switch to changes, open a file
// there, and switch back and forth with tab.
// Goal: each tab keeps its own open file.
func TestTabsKeepTheirPlace(t *testing.T) {
	m := newTestModel(t)

	// Log tab: second commit, focus its files, open a.go.
	press(m, "2")
	press(m, "down")
	press(m, "enter")
	press(m, "enter")
	if m.log.files.viewer == nil || m.log.files.viewer.file.Change.Path() != "a.go" {
		t.Fatalf("log viewer not open on a.go")
	}

	// Changes tab: nothing open yet, open b.go.
	press(m, "tab")
	if m.tab != tabChanges || m.changes.viewer != nil {
		t.Fatalf("changes tab should start on its file table")
	}
	press(m, "enter")
	if m.changes.viewer == nil || m.changes.viewer.file.Change.Path() != "b.go" {
		t.Fatalf("changes viewer not open on b.go")
	}

	// Back and forth: both viewers are still there.
	press(m, "tab")
	if m.tab != tabLog || m.log.files.viewer == nil || m.log.files.viewer.file.Change.Path() != "a.go" {
		t.Errorf("log tab lost its open file")
	}
	press(m, "1")
	if m.changes.viewer == nil {
		t.Errorf("changes tab lost its open file")
	}
}

// Scenario: move quickly through several commits.
// Goal: only the commit the cursor stops on is loaded; stale ticks are
// dropped.
func TestLoadOnlyWhereTheCursorStops(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")

	// Move twice without running the tick commands, then run only the
	// first (stale) tick and the last one.
	first := m.logKey("down")
	last := m.logKey("down")

	run(m, first)
	if m.log.loading != "" || len(m.log.cache) != 1 {
		t.Errorf("stale tick started a load: loading=%q cached=%d", m.log.loading, len(m.log.cache))
	}

	run(m, last)
	if m.log.files.set == nil || m.log.files.set.list.visible[0].Change.Status != git.Added {
		t.Errorf("oldest commit should show a.go as added: %+v", m.log.files.set)
	}
}

// Scenario: toggle the comment filter while the log tab shows a commit.
// Goal: the shown commit is recomputed with the new filters.
func TestFilterToggleRecomputesShownCommit(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")

	press(m, "c")
	if m.log.files.set.filters != m.filters || m.changes.set.filters != m.filters {
		t.Errorf("shown sets not recomputed after filter toggle")
	}
}
