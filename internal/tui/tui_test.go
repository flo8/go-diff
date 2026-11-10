package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
//
// With prefetch, the history rows are loaded the way the app does at
// startup; without, nothing is loaded yet.
func newTestModel(t *testing.T, prefetch bool) *Model {
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
	m.resultDelay = 0
	m.refreshEvery = 0 // tests send autoRefreshTick themselves
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if prefetch {
		run(m, cmd)
	}
	return m
}

// reopen builds a new model on the same repository, as starting godiff
// again would, to pick up changes made to the files.
func reopen(t *testing.T, old *Model) *Model {
	rg, err := old.repo.ResolveRange(git.Options{})
	if err != nil {
		t.Fatal(err)
	}
	files, err := review.Load(old.repo, rg)
	if err != nil {
		t.Fatal(err)
	}
	review.ComputeAll(files, semantic.DefaultFilters)

	m := New(Options{
		Repo: old.repo, Files: files, Range: rg.String(),
		Filters: semantic.DefaultFilters, Themes: theme.Builtins(), Theme: "dark",
	})
	m.resultDelay = 0
	send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return m
}

// press sends a key and runs the commands it returns, and the commands
// those return, the way the Bubble Tea loop would.
func press(m *Model, key string) {
	run(m, m.handleKey(key))
}

// run runs a command and everything it leads to. A batch runs its
// commands one after the other, in order.
func run(m *Model, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				run(m, c)
			}
			return
		}
		_, cmd = m.Update(msg)
	}
}

// send passes a message to Update and runs what it returns.
func send(m *Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	run(m, cmd)
}

// isolateGit makes the git commands godiff runs itself (commit, push)
// use a fixed identity and ignore the user's config, hooks included.
func isolateGit(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
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
	m := newTestModel(t, true)

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
	m := newTestModel(t, true)

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
	m := newTestModel(t, true)

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
	m := newTestModel(t, true)

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

// Scenario: the file table with and without the history panel.
// Goal: the totals line is drawn only when the history (which shows the
// totals in its rows) is hidden.
func TestTotalsOnlyWithoutHistory(t *testing.T) {
	m := newTestModel(t, true)

	if strings.Contains(ansi.Strip(m.mainView()), "Total:") {
		t.Errorf("totals line should be hidden while the history is shown")
	}

	press(m, "L")
	if !strings.Contains(ansi.Strip(m.mainView()), "Total: 1 file") {
		t.Errorf("totals line should be shown when the history is hidden")
	}
}

// Scenario: nothing loaded yet, the cursor on the second commit.
// Goal: the selected commit is loaded first, then the other visible
// ones, without duplicates and at most n; with "current" selected, the
// visible commits from the top; a stale tick starts nothing.
func TestLoadOrder(t *testing.T) {
	m := newTestModel(t, false)
	m.hist.loading = map[string]bool{} // the startup loads were not run
	c := m.hist.commits

	m.hist.selected = 2
	got := m.wantedCommits(maxLoads)
	want := []string{c[1].Hash, c[0].Hash, c[2].Hash}
	if !slices.Equal(got, want) {
		t.Errorf("wanted %v, want selected first then visible %v", got, want)
	}
	if got := m.wantedCommits(1); !slices.Equal(got, want[:1]) {
		t.Errorf("wanted %v, want only the selected commit", got)
	}

	m.hist.selected = 0
	m.hist.loading[c[0].Hash] = true
	if got := m.wantedCommits(maxLoads); !slices.Equal(got, []string{c[1].Hash, c[2].Hash}) {
		t.Errorf("wanted %v, want the visible commits not already loading", got)
	}

	m.hist.seq = 5
	if cmd := m.startLoad(loadTick{seq: 4}); cmd != nil {
		t.Errorf("stale tick should not start a load")
	}
}

// Scenario: the history rows before and after their commits load.
// Goal: dots while unknown, then the same totals as the file table.
func TestHistoryRowTotals(t *testing.T) {
	m := newTestModel(t, false)
	if row := ansi.Strip(m.historyRow(2)); !strings.Contains(row, "·") {
		t.Errorf("unloaded row should show dots: %q", row)
	}

	m = newTestModel(t, true)
	if len(m.hist.cache) != 3 {
		t.Fatalf("startup should load the 3 visible commits, got %d", len(m.hist.cache))
	}
	if got := m.entrySet(2).totals(); got.Modified != 1 || got.Added != 0 || got.Removed != 0 {
		t.Errorf("change a totals = %+v, want 1 modified", got)
	}
	if got := m.entrySet(0).totals(); got.Modified != 1 {
		t.Errorf("current totals = %+v, want 1 modified", got)
	}
	row := ansi.Strip(m.historyRow(2))
	if !strings.Contains(row, "    1 file     0     0     1  change a") {
		t.Errorf("row = %q", row)
	}
}

// Scenario: toggle the comment filter (F then c) while a commit is shown.
// Goal: the filter flips and the shown commit and "current" are
// recomputed.
func TestFilterToggleRecomputesShown(t *testing.T) {
	m := newTestModel(t, true)
	press(m, "tab")
	press(m, "down")

	press(m, "F")
	press(m, "c")
	if m.filters.Comments {
		t.Errorf("F c should turn the comment filter off")
	}
	if m.files.set.filters != m.filters || m.current.filters != m.filters {
		t.Errorf("shown sets not recomputed after filter toggle")
	}
}

// Scenario: a working tree with three changed files: b.go (1 changed
// line), c.go (4) and d.go (2); then o, then a commit shown after o.
// Goal: the table starts sorted by changed lines (c, d, b); o sorts by
// name, and other history entries follow the same order.
func TestSortByChanges(t *testing.T) {
	m := newTestModel(t, true)
	dir := m.repo.Root
	os.WriteFile(filepath.Join(dir, "c.go"), []byte("package a\n\nvar c = 1\nvar d = 2\nvar e = 3\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "d.go"), []byte("package a\n\nvar f = 1\n"), 0o644)
	m = reopen(t, m)

	if got := strings.Join(selectedPaths(m), " "); got != "c.go d.go b.go" {
		t.Errorf("by changes: %s, want c.go d.go b.go", got)
	}

	press(m, "o")
	if got := strings.Join(selectedPaths(m), " "); got != "b.go c.go d.go" {
		t.Errorf("by name: %s, want b.go c.go d.go", got)
	}

	// A commit shown after o is sorted by name too.
	press(m, "tab")
	press(m, "down")
	if m.files.set.list.sortByChanges {
		t.Errorf("commit tables should follow o")
	}
}

// Scenario: F followed by a key that is not a filter.
// Goal: nothing changes and the next key works normally again.
func TestFilterKeyCancel(t *testing.T) {
	m := newTestModel(t, true)
	before := m.filters

	press(m, "F")
	press(m, "esc")
	if m.filters != before || m.filterKey {
		t.Errorf("esc after F should change nothing")
	}
}
