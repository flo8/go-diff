package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
