package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	wheelLines  = 3                      // how far one wheel notch scrolls the diff viewer
	doubleClick = 400 * time.Millisecond // most time between the two clicks of a double-click
)

// fileClick remembers the last click on a file. Bubble Tea reports
// single clicks only, so a double-click is a second click on the same
// file of the same table soon after.
type fileClick struct {
	at   time.Time
	set  *changeSet
	file int
}

// mouseClick selects the file or history entry under a left click and
// gives its panel the focus. Clicks elsewhere do nothing.
func (m *Model) mouseClick(msg tea.MouseClickMsg) tea.Cmd {
	var i int
	var ok bool

	if msg.Button != tea.MouseLeft || m.input != nil || m.help || m.files.viewer != nil {
		return nil
	}

	// A file of the table above: select it, or open it on a double-click.
	i, ok = m.fileAt(msg.Y)
	if ok {
		m.files.set.list.selected = i
		m.focus = focusFiles
		m.clickFile(i)
		return nil
	}

	// An entry of the history below.
	i, ok = m.historyEntryAt(msg.Y)
	if ok {
		m.focus = focusHistory
		return m.selectEntry(i)
	}

	return nil
}

// clickFile opens file i of the shown table when the previous click was
// on the same file less than doubleClick ago.
func (m *Model) clickFile(i int) {
	var now time.Time
	var prev fileClick

	now = time.Now()
	prev = m.lastClick
	m.lastClick = fileClick{at: now, set: m.files.set, file: i}

	if prev.set == m.files.set && prev.file == i && now.Sub(prev.at) < doubleClick {
		m.lastClick = fileClick{}
		m.openSelected(&m.files)
	}
}

// mouseWheel scrolls what is under the mouse: the diff viewer, the
// history or the file table. In the panels it moves the selection, as
// the arrow keys do, without moving the focus.
func (m *Model) mouseWheel(msg tea.MouseWheelMsg) tea.Cmd {
	var step int

	switch msg.Button {
	case tea.MouseWheelUp:
		step = -1
	case tea.MouseWheelDown:
		step = 1
	default:
		return nil
	}

	if m.input != nil || m.help {
		return nil
	}

	switch {
	case m.files.viewer != nil:
		m.files.viewer.top += step * wheelLines
		m.files.viewer.clamp(m.height)
	case m.overHistory(msg.Y):
		return m.selectEntry(m.hist.selected + step)
	case m.files.set != nil:
		m.files.set.list.move(step)
	}

	return nil
}

// overHistory reports whether screen line y is in the history panel,
// its rule included.
func (m *Model) overHistory(y int) bool {
	return m.showHistory && y > m.tableHeight()
}

// fileAt returns the index in the visible list of the file drawn on
// screen line y, or false when no file is drawn there. The file table
// starts below the header, with its titles and rule; group separators
// are not files.
func (m *Model) fileAt(y int) (int, bool) {
	var first int
	var rows int
	var lines []tableLine
	var line tableLine

	if m.files.set == nil {
		return 0, false
	}

	first = 1 + tableChrome
	rows = tableRows(m.tableHeight(), !m.showHistory)
	lines = m.files.set.list.bodyLines(rows)
	if y < first || y >= first+len(lines) {
		return 0, false
	}

	line = lines[y-first]
	if line.separator {
		return 0, false
	}
	return line.file, true
}

// historyEntryAt returns the history entry drawn on screen line y, or
// false when no entry is drawn there. It follows mainView and
// historyPanel: the header, the file table, the history rule, an error
// line when reading the history failed, then the entries.
func (m *Model) historyEntryAt(y int) (int, bool) {
	var first int
	var i int

	if !m.showHistory {
		return 0, false
	}

	first = 1 + m.tableHeight() + 1
	if m.hist.err != nil {
		first++
	}

	// Below the last row drawn, or past the last entry, is empty space.
	i = m.hist.top + y - first
	if y < first || y >= first+m.historyRows() || i >= m.hist.entries() {
		return 0, false
	}

	return i, true
}
