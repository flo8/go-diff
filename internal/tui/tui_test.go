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

// selectedPaths lists the visible files of the shown table.
func selectedPaths(m *Model) []string {
	var paths []string
	if m.files.set == nil {
		return nil
	}
	for _, f := range m.files.set.list.visible {
		paths = append(paths, f.Change.Path())
	}
	return paths
}

// Scenario: start godiff with unstaged work in b.go.
// Goal: the history lists "current" then the three commits, "current"
// is selected, and the file table shows the working tree change.
func TestStartsOnCurrent(t *testing.T) {
	m := newTestModel(t)

	if m.hist.entries() != 4 || m.hist.selected != 0 {
		t.Fatalf("entries=%d selected=%d, want 4 entries with current selected", m.hist.entries(), m.hist.selected)
	}
	if m.hist.commits[0].Subject != "add b" {
		t.Errorf("first commit = %q, want newest (add b)", m.hist.commits[0].Subject)
	}
	if m.files.set != m.current || m.current.list.visible[0].Counts.Modified != 1 {
		t.Errorf("current should show b.go with 1 modified line: %v", selectedPaths(m))
	}

	view := m.mainView()
	if !strings.Contains(view, "current") || !strings.Contains(view, "History") {
		t.Errorf("main view should show the history panel with a current row")
	}
}

// Scenario: move the focus to the history and walk down and back up.
// Goal: each commit shows its own files and counts; back on "current"
// the working tree change shows again.
func TestHistoryShowsEachCommitsFiles(t *testing.T) {
	m := newTestModel(t)

	press(m, "tab")
	if m.focus != focusHistory {
		t.Fatalf("tab should focus the history")
	}

	// "add b": b.go created with 2 code lines.
	press(m, "down")
	if p := selectedPaths(m); len(p) != 1 || p[0] != "b.go" || m.files.set.list.visible[0].Counts.Added != 2 {
		t.Errorf("add b files = %v", p)
	}

	// "change a": a.go with one modified line.
	press(m, "down")
	if p := selectedPaths(m); len(p) != 1 || p[0] != "a.go" || m.files.set.list.visible[0].Counts.Modified != 1 {
		t.Errorf("change a files = %v", p)
	}

	// Back to "current".
	press(m, "g")
	if m.files.set != m.current {
		t.Errorf("g should go back to current")
	}
}

// Scenario: open a file of a commit, leave the viewer.
// Goal: Esc returns to the split with the same commit selected and the
// file table focused.
func TestOpenFileFromCommit(t *testing.T) {
	m := newTestModel(t)

	press(m, "tab")
	press(m, "down")
	press(m, "down")
	press(m, "enter") // focus the files
	press(m, "enter") // open a.go
	if m.files.viewer == nil || m.files.viewer.file.Change.Path() != "a.go" {
		t.Fatalf("viewer not open on a.go")
	}
	if !strings.Contains(m.viewerView(&m.files), "^: a.go") {
		t.Errorf("side titles should name the parent commit")
	}

	// tab does nothing while a file is open.
	press(m, "tab")
	if m.files.viewer == nil {
		t.Errorf("tab should not close the viewer")
	}

	press(m, "esc")
	if m.files.viewer != nil || m.hist.selected != 2 || m.focus != focusFiles {
		t.Errorf("esc should return to the table: viewer=%v selected=%d focus=%d", m.files.viewer != nil, m.hist.selected, m.focus)
	}
}

// Scenario: hide and show the history panel.
// Goal: hiding moves the focus to the files; tab shows it again and
// focuses it.
func TestToggleHistory(t *testing.T) {
	m := newTestModel(t)

	press(m, "tab")
	press(m, "L")
	if m.showHistory || m.focus != focusFiles {
		t.Errorf("L should hide the history and focus the files")
	}
	if strings.Contains(m.mainView(), "History") {
		t.Errorf("hidden history should not be drawn")
	}

	press(m, "tab")
	if !m.showHistory || m.focus != focusHistory {
		t.Errorf("tab should show and focus the history")
	}
}

// Scenario: move quickly through two commits.
// Goal: only the commit the cursor stops on is loaded; the stale tick
// is dropped.
func TestLoadOnlyWhereTheCursorStops(t *testing.T) {
	m := newTestModel(t)
	press(m, "tab")

	// Move twice without running the tick commands, then run the first
	// (stale) tick and the last one.
	first := m.historyKey("down")
	last := m.historyKey("down")

	run(m, first)
	if m.hist.loading != "" || len(m.hist.cache) != 0 {
		t.Errorf("stale tick started a load: loading=%q cached=%d", m.hist.loading, len(m.hist.cache))
	}

	run(m, last)
	if p := selectedPaths(m); len(p) != 1 || p[0] != "a.go" || len(m.hist.cache) != 1 {
		t.Errorf("change a should be loaded alone: %v cached=%d", p, len(m.hist.cache))
	}
}

// Scenario: toggle the comment filter while a commit is shown.
// Goal: the shown commit and "current" are recomputed.
func TestFilterToggleRecomputesShown(t *testing.T) {
	m := newTestModel(t)
	press(m, "tab")
	press(m, "down")

	press(m, "c")
	if m.files.set.filters != m.filters || m.current.filters != m.filters {
		t.Errorf("shown sets not recomputed after filter toggle")
	}
}
