package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// writeFiles writes files with n lines each in the test repository.
func writeFiles(t *testing.T, dir string, lines map[string]int) {
	for name, n := range lines {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "line %d\n", i)
		}
		err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// tableText draws the shown file table as plain text.
func tableText(m *Model, height int) []string {
	var lines []string
	for _, l := range m.fileTable(m.files.set, 120, height, true, false) {
		lines = append(lines, ansi.Strip(l))
	}
	return lines
}

// Scenario: a working tree with b.go (1 changed line), c.go (5 new),
// z_test.go (10 new) and notes.md (3 new); then O, then o.
// Goal: grouping is off at first; O puts go files, then tests, then
// other files, keeping the sort order inside each group, and o still
// switches that order.
func TestGroupByKindOrder(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)

	if got := strings.Join(selectedPaths(m), " "); got != "z_test.go c.go notes.md b.go" {
		t.Errorf("ungrouped: %s, want z_test.go c.go notes.md b.go", got)
	}

	press(m, "O")
	if got := strings.Join(selectedPaths(m), " "); got != "c.go b.go z_test.go notes.md" {
		t.Errorf("grouped by changes: %s, want c.go b.go z_test.go notes.md", got)
	}

	press(m, "o")
	if got := strings.Join(selectedPaths(m), " "); got != "b.go c.go z_test.go notes.md" {
		t.Errorf("grouped by name: %s, want b.go c.go z_test.go notes.md", got)
	}

	press(m, "O")
	if m.files.set.list.grouped {
		t.Errorf("second O should turn grouping off")
	}
}

// Scenario: the same working tree, grouped, drawn in a tall table.
// Goal: each group has a separator with its file count right above its
// first file; an ungrouped table has none.
func TestGroupSeparators(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)

	for _, l := range tableText(m, 20) {
		if strings.Contains(l, "──") && strings.Contains(l, "(") {
			t.Errorf("ungrouped table has a separator: %q", l)
		}
	}

	press(m, "O")
	lines := tableText(m, 20)
	want := []string{"go (2)", "c.go", "b.go", "tests (1)", "z_test.go", "other (1)", "notes.md"}

	// Rows 0 and 1 are the titles and the rule.
	for i, w := range want {
		if !strings.Contains(lines[i+2], w) {
			t.Errorf("row %d: %q, want it to contain %q", i, lines[i+2], w)
		}
	}
}

// Scenario: 15 files, 5 in each group, grouped, in a table with room
// for only 6 rows; the selection walks down then up the whole list.
// Goal: the selected file is always drawn, even with separators taking
// rows, and the table never grows past its height.
func TestGroupScrollKeepsSelection(t *testing.T) {
	m := newTestModel(t, true)
	files := map[string]int{}
	for i := range 5 {
		files[fmt.Sprintf("g%d.go", i)] = 2
		files[fmt.Sprintf("g%d_test.go", i)] = 2
		files[fmt.Sprintf("g%d.txt", i)] = 2
	}
	writeFiles(t, m.repo.Root, files)
	m = reopen(t, m)
	press(m, "O")

	keys := append(strings.Split(strings.Repeat("j", 16), ""), strings.Split(strings.Repeat("k", 16), "")...)
	for step, key := range keys {
		press(m, key)
		lines := tableText(m, 8)
		if len(lines) != 8 {
			t.Fatalf("step %d: %d lines, want 8", step, len(lines))
		}
		selected := m.files.set.list.visible[m.files.set.list.selected].Change.Path()
		if !strings.Contains(strings.Join(lines, "\n"), "▸") {
			t.Errorf("step %d: selected %s not drawn", step, selected)
		}
	}
}
