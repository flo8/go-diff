package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// screenLine returns the index of the first line of the main screen
// that contains text, or fails the test.
func screenLine(t *testing.T, m *Model, text string) int {
	t.Helper()
	lines := strings.Split(m.mainView(), "\n")
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), text) {
			return i
		}
	}
	t.Fatalf("%q is not on screen", text)
	return 0
}

// click sends a left click on screen line y.
func click(m *Model, y int) {
	send(m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
}

// Scenario: the file table has the focus; click the "change a" row of
// the history, found where the screen draws it.
// Goal: the click selects that commit, shows its files and moves the
// focus to the history.
func TestClickSelectsHistoryEntry(t *testing.T) {
	m := newTestModel(t, true)

	click(m, screenLine(t, m, "change a"))

	commit, ok := m.hist.selectedCommit()
	if !ok || commit.Subject != "change a" {
		t.Fatalf("selected %+v, want \"change a\"", commit)
	}
	if m.focus != focusHistory {
		t.Errorf("focus = %v, want history", m.focus)
	}
	if got := selectedPaths(m); len(got) != 1 || got[0] != "a.go" {
		t.Errorf("files = %v, want [a.go]", got)
	}
}

// Scenario: an error's output is shown above the footer, which makes
// the file table shorter and moves the history up.
// Goal: a click still lands on the row the screen draws.
func TestClickWithErrorOutput(t *testing.T) {
	m := newTestModel(t, true)
	m.result = &result{err: true, text: "push failed", detail: []string{"line 1", "line 2", "line 3"}}

	click(m, screenLine(t, m, "add a"))

	commit, ok := m.hist.selectedCommit()
	if !ok || commit.Subject != "add a" {
		t.Fatalf("selected %+v, want \"add a\"", commit)
	}
}

// Scenario: click the file table, the history rule, and empty space
// below the last history entry.
// Goal: none of them change the selection or the focus.
func TestClickOutsideHistoryEntries(t *testing.T) {
	m := newTestModel(t, true)
	rule := screenLine(t, m, "History")
	last := screenLine(t, m, "add a")

	for _, y := range []int{0, 3, rule, last + 1} {
		click(m, y)
		if m.hist.selected != 0 || m.focus != focusFiles {
			t.Fatalf("click on line %d: selected %d, focus %v", y, m.hist.selected, m.focus)
		}
	}
}

// Scenario: roll the wheel down twice over the history, then up once.
// Goal: the selection follows one entry per notch and the file table
// shows the selected commit, without taking the focus.
func TestWheelOverHistory(t *testing.T) {
	m := newTestModel(t, true)
	y := screenLine(t, m, "History")

	send(m, tea.MouseWheelMsg{Y: y, Button: tea.MouseWheelDown})
	send(m, tea.MouseWheelMsg{Y: y, Button: tea.MouseWheelDown})
	send(m, tea.MouseWheelMsg{Y: y, Button: tea.MouseWheelUp})

	commit, ok := m.hist.selectedCommit()
	if !ok || commit.Subject != "add b" {
		t.Fatalf("selected %+v, want \"add b\"", commit)
	}
	if m.focus != focusFiles {
		t.Errorf("focus = %v, want files", m.focus)
	}
}

// Scenario: roll the wheel over the file table.
// Goal: the history selection does not move.
func TestWheelOverFileTable(t *testing.T) {
	m := newTestModel(t, true)

	send(m, tea.MouseWheelMsg{Y: 3, Button: tea.MouseWheelDown})

	if m.hist.selected != 0 {
		t.Errorf("history selected %d, want 0", m.hist.selected)
	}
}

// selectedFile returns the path of the selected file of the shown table.
func selectedFile(m *Model) string {
	l := m.files.set.list
	return l.visible[l.selected].Change.Path()
}

// Scenario: a grouped table with c.go, b.go, z_test.go and notes.md,
// and the focus on the history; click the notes.md row, found where the
// screen draws it.
// Goal: notes.md is selected, below two group separators, and the file
// table gets the focus.
func TestClickSelectsFile(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)
	press(m, "tab")

	click(m, screenLine(t, m, "notes.md"))

	if got := selectedFile(m); got != "notes.md" {
		t.Errorf("selected %s, want notes.md", got)
	}
	if m.focus != focusFiles {
		t.Errorf("focus = %v, want files", m.focus)
	}
}

// Scenario: the same table; click the "tests" group separator, then
// the column titles.
// Goal: neither is a file, so the selection does not move.
func TestClickFileTableNotAFile(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)
	before := selectedFile(m)

	click(m, screenLine(t, m, "tests (1)"))
	click(m, 1)

	if got := selectedFile(m); got != before {
		t.Errorf("selected %s, want %s", got, before)
	}
}

// Scenario: the history is hidden, so the table is taller and ends with
// its totals; click the z_test.go row.
// Goal: the click still lands on the row the screen draws.
func TestClickFileWithoutHistory(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)
	press(m, "L")

	click(m, screenLine(t, m, "z_test.go"))

	if got := selectedFile(m); got != "z_test.go" {
		t.Errorf("selected %s, want z_test.go", got)
	}
}

// Scenario: click the notes.md row twice in a row.
// Goal: the second click opens notes.md in the diff viewer.
func TestDoubleClickOpensFile(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)
	y := screenLine(t, m, "notes.md")

	click(m, y)
	click(m, y)

	if m.files.viewer == nil {
		t.Fatal("double-click did not open the file")
	}
	if got := m.files.viewer.file.Change.Path(); got != "notes.md" {
		t.Errorf("opened %s, want notes.md", got)
	}
}

// Scenario: click notes.md, wait longer than a double-click (the first
// click is moved back in time), click it again; then click c.go right
// after notes.md.
// Goal: neither pair opens a file: too slow, then two different files.
func TestSlowOrDifferentClicksDoNotOpen(t *testing.T) {
	m := newTestModel(t, true)
	writeFiles(t, m.repo.Root, map[string]int{"c.go": 5, "z_test.go": 10, "notes.md": 3})
	m = reopen(t, m)
	notes := screenLine(t, m, "notes.md")
	cgo := screenLine(t, m, "c.go")

	click(m, notes)
	m.lastClick.at = m.lastClick.at.Add(-time.Second)
	click(m, notes)
	if m.files.viewer != nil {
		t.Fatal("slow clicks opened the file")
	}

	click(m, cgo)
	if m.files.viewer != nil {
		t.Fatal("clicks on two files opened one")
	}
}
