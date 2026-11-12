package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/git"
)

// newPackagesModel adds new files in three directories to the test
// working tree, which already has b.go changed at the top:
//
//	internal/tui/x.go, internal/tui/x_test.go   one package, code and test
//	cmd/godiff/main.go                          a second package
//	notes.md                                    not Go: no package
//
// The history is hidden so the file table shows its totals.
func newPackagesModel(t *testing.T) *Model {
	m := newTestModel(t, true)
	for _, dir := range []string{"internal/tui", "cmd/godiff"} {
		err := os.MkdirAll(filepath.Join(m.repo.Root, dir), 0o755)
		if err != nil {
			t.Fatal(err)
		}
	}
	writeFiles(t, m.repo.Root, map[string]int{
		"internal/tui/x.go": 5, "internal/tui/x_test.go": 10, "cmd/godiff/main.go": 2, "notes.md": 3,
	})
	m = reopen(t, m)
	press(m, "L")
	return m
}

// Scenario: the working tree above.
// Goal: 3 packages: internal/tui (code and test count once),
// cmd/godiff, and "." for b.go; notes.md adds none. The totals row
// says so.
func TestPackageTotal(t *testing.T) {
	m := newPackagesModel(t)

	if got := m.current.packageTotal(); got != 3 {
		t.Errorf("packages = %d, want 3", got)
	}
	if !strings.Contains(ansi.Strip(m.mainView()), "Total: 5 files, 3 packages") {
		t.Errorf("totals row should say 5 files, 3 packages")
	}
}

// Scenario: the same working tree drawn on screen.
// Goal: each row shows the directory in the Package column and only the
// base name in the Filename column.
func TestPackageColumn(t *testing.T) {
	m := newPackagesModel(t)
	screen := ansi.Strip(m.mainView())

	for _, want := range []string{"Package", "internal/tui  x_test.go", "cmd/godiff    main.go", ".             notes.md"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
}

// Scenario: renames inside one directory and across directories.
// Goal: the old name keeps its directory only when the file moved.
func TestDisplayNameRenames(t *testing.T) {
	cases := []struct {
		change git.Change
		want   string
	}{
		{git.Change{Status: git.Modified, NewPath: "internal/tui/model.go"}, "model.go"},
		{git.Change{Status: git.Renamed, OldPath: "internal/tui/a.go", NewPath: "internal/tui/b.go"}, "a.go → b.go"},
		{git.Change{Status: git.Renamed, OldPath: "internal/old/a.go", NewPath: "internal/tui/a.go"}, "internal/old/a.go → a.go"},
	}

	for _, c := range cases {
		if got := displayName(c.change, 80); got != c.want {
			t.Errorf("displayName(%s) = %q, want %q", c.change.NewPath, got, c.want)
		}
	}
}

// Scenario: a directory longer than its column.
// Goal: it is cut from the left so its last part stays visible.
func TestCutLeft(t *testing.T) {
	if got := cutLeft("internal/semantic", 8); got != "…emantic" {
		t.Errorf("cutLeft = %q, want …emantic", got)
	}
	if got := cutLeft("cmd", 8); got != "cmd" {
		t.Errorf("cutLeft = %q, want cmd", got)
	}
}

// clickPackageOf clicks dir in the Package column of the row that ends
// with file name, at the column where the screen draws it.
func clickPackageOf(t *testing.T, m *Model, dir, name string) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.mainView()), "\n") {
		if !strings.HasSuffix(strings.TrimRight(line, " "), " "+name) {
			continue
		}
		i := strings.Index(line, " "+dir+" ")
		if i < 0 {
			t.Fatalf("row of %s has no %s: %q", name, dir, line)
		}
		x := ansi.StringWidth(line[:i+1])
		send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		return
	}
	t.Fatalf("no row for %s", name)
}

// Scenario: click "internal/tui" in the Package column of the x.go row.
// Goal: the rows are x.go and x_test.go only, the header names the
// package, and the totals still count all 5 files and 3 packages.
func TestClickPackageFilters(t *testing.T) {
	m := newPackagesModel(t)

	clickPackageOf(t, m, "internal/tui", "x.go")

	if got := strings.Join(selectedPaths(m), " "); got != "internal/tui/x.go internal/tui/x_test.go" {
		t.Errorf("rows = %s, want internal/tui/x.go internal/tui/x_test.go", got)
	}
	screen := ansi.Strip(m.mainView())
	if !strings.Contains(screen, "(package internal/tui)") {
		t.Errorf("header should name the package")
	}
	if !strings.Contains(screen, "Total: 5 files, 3 packages") {
		t.Errorf("totals should still count every file")
	}
}

// Scenario: filter on internal/tui, click the same package again (fast,
// like a double-click), filter again, then press Esc.
// Goal: the second click and Esc both show all 5 files again, and the
// two quick clicks do not open a file.
func TestClearPackageFilter(t *testing.T) {
	m := newPackagesModel(t)

	clickPackageOf(t, m, "internal/tui", "x.go")
	clickPackageOf(t, m, "internal/tui", "x.go")
	if got := len(selectedPaths(m)); got != 5 {
		t.Errorf("after a second click: %d rows, want 5", got)
	}
	if m.files.viewer != nil {
		t.Errorf("clicks on a package should not open the file")
	}

	clickPackageOf(t, m, "cmd/godiff", "main.go")
	press(m, "esc")
	if got := len(selectedPaths(m)); got != 5 {
		t.Errorf("after esc: %d rows, want 5", got)
	}
}

// Scenario: filter on internal/tui, then select the "add b" commit,
// which only touches b.go at the top of the repository.
// Goal: the filter stays on and the table says the package has no
// changes there; the history row of "current" still counts 5 files.
func TestPackageFilterFollowsHistory(t *testing.T) {
	m := newPackagesModel(t)
	clickPackageOf(t, m, "internal/tui", "x.go")

	press(m, "L")
	press(m, "tab")
	press(m, "down")

	if !strings.Contains(ansi.Strip(m.mainView()), "No changes in internal/tui.") {
		t.Errorf("table should say the package has no changes in this commit")
	}
	if row := ansi.Strip(m.historyRow(0)); !strings.Contains(row, "5 files") {
		t.Errorf("current row = %q, want 5 files", row)
	}
}

// Scenario: click the file name x.go, right after the Package column.
// Goal: only the Package column filters: all 5 rows stay.
func TestClickFileNameDoesNotFilter(t *testing.T) {
	m := newPackagesModel(t)

	for y, line := range strings.Split(ansi.Strip(m.mainView()), "\n") {
		if i := strings.Index(line, "  x.go"); i >= 0 {
			send(m, tea.MouseClickMsg{X: ansi.StringWidth(line[:i+2]), Y: y, Button: tea.MouseLeft})
		}
	}

	if m.pkgFilter != "" || len(selectedPaths(m)) != 5 {
		t.Errorf("filter = %q with %d rows, want none and 5", m.pkgFilter, len(selectedPaths(m)))
	}
}
