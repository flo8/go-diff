package tui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
)

// Column widths of the file table.
const (
	numColWidth     = 9
	detailsColWidth = 14
	tableChrome     = 2 // column titles and rule
)

// fileList is the state of a file table.
type fileList struct {
	visible       []*review.File // files with meaningful changes, sorted
	hidden        int            // files changed only in ignored ways
	sortByChanges bool
	grouped       bool            // go, tests, other: see groups.go
	groupFiles    [groupCount]int // number of visible files in each group
	selected      int
	top           int // first visible row when the list scrolls
}

// refresh rebuilds the visible list, keeping the selected file if it is
// still there.
func (l *fileList) refresh(files []*review.File) {
	var keep *review.File

	if l.selected < len(l.visible) {
		keep = l.visible[l.selected]
	}

	l.visible = l.visible[:0]
	for _, f := range files {
		if f.Meaningful() {
			l.visible = append(l.visible, f)
		}
	}
	l.hidden = len(files) - len(l.visible)
	l.sort()

	// Count the files of each group for the separator lines.
	l.groupFiles = [groupCount]int{}
	for _, f := range l.visible {
		l.groupFiles[groupOf(f.Change.Path())]++
	}

	// Find the previously selected file again.
	l.selected = 0
	for i, f := range l.visible {
		if f == keep {
			l.selected = i
		}
	}
}

// sort orders by path, or by number of changes (most first); when
// grouped, by group first.
func (l *fileList) sort() {
	sort.SliceStable(l.visible, l.less)
}

func (l *fileList) less(i, j int) bool {
	var gi fileGroup
	var gj fileGroup

	// Groups come first, in their order.
	if l.grouped {
		gi = groupOf(l.visible[i].Change.Path())
		gj = groupOf(l.visible[j].Change.Path())
		if gi != gj {
			return gi < gj
		}
	}

	if l.sortByChanges {
		return l.byChanges(i, j)
	}
	return l.byPath(i, j)
}

func (l *fileList) byPath(i, j int) bool {
	return l.visible[i].Change.Path() < l.visible[j].Change.Path()
}

func (l *fileList) byChanges(i, j int) bool {
	a := l.visible[i].Counts.Total()
	b := l.visible[j].Counts.Total()
	if a != b {
		return a > b
	}
	return l.byPath(i, j)
}

// move changes the selection by delta rows.
func (l *fileList) move(delta int) {
	l.selected = min(max(l.selected+delta, 0), max(len(l.visible)-1, 0))
}

// scroll keeps the selected row inside a window of height rows.
func (l *fileList) scroll(height int) {
	if l.selected < l.top {
		l.top = l.selected
	}
	if l.selected >= l.top+height {
		l.top = l.selected - height + 1
	}
}

// fileListKey handles the keys of a focused file table.
func (m *Model) fileListKey(p *pane, key string) {
	var l *fileList
	var page int

	l = &p.set.list
	page = max(m.height-2-tableChrome, 1)

	switch key {
	case "up", "k":
		l.move(-1)
	case "down", "j":
		l.move(1)
	case "pgup", "ctrl+b":
		l.move(-page)
	case "pgdown", "ctrl+f", "space":
		l.move(page)
	case "home", "g":
		l.move(-len(l.visible))
	case "end", "G":
		l.move(len(l.visible))
	case "o":
		m.sortByChanges = !m.sortByChanges
		m.applySort(p.set)
	case "O":
		m.groupByKind = !m.groupByKind
		m.applySort(p.set)
	case "enter", "l", "right":
		m.openSelected(p)
	}
}

// openSelected opens the diff viewer on the selected file of a pane.
func (m *Model) openSelected(p *pane) {
	if p.set == nil || len(p.set.list.visible) == 0 {
		return
	}
	p.viewer = newViewer(p.set.list.visible[p.set.list.selected], m.height)
}

// fileTable draws a file table in exactly height lines of width columns:
// titles, a rule and the files, then a rule and the totals when
// withTotals is set (the history panel shows them otherwise). Without
// focus the selected row is marked but not highlighted.
func (m *Model) fileTable(s *changeSet, width, height int, focused, withTotals bool) []string {
	var lines []string
	var footer int
	var rows int
	var staging bool
	var f *review.File
	var g fileGroup

	staging = s == m.current && m.canStage && m.repo != nil
	footer = 0
	if withTotals {
		footer = 2
	}
	rows = max(height-2-footer, 1)

	// Separators take rows too: keep room for one per group so the
	// selected file always stays on screen.
	if s.list.grouped {
		s.list.scroll(max(rows-int(groupCount), 1))
	} else {
		s.list.scroll(rows)
	}

	lines = append(lines, m.tableTitles(width, staging))
	lines = append(lines, m.rule(width))

	for i := s.list.top; i < len(s.list.visible) && len(lines) < rows+2; i++ {
		f = s.list.visible[i]

		// A grouped table names each group above its first file.
		if s.list.grouped && s.list.startsGroup(i) {
			g = groupOf(f.Change.Path())
			lines = append(lines, m.groupSeparator(g, s.list.groupFiles[g], width))
		}
		if len(lines) < rows+2 {
			lines = append(lines, m.fileRow(f, staging, i == s.list.selected, focused, width))
		}
	}

	// An empty list says why.
	if len(s.list.visible) == 0 {
		lines = append(lines, m.pal.Paint("  No meaningful changes.", m.pal.Muted))
	}

	// Fill the space; the totals, if any, stay at the bottom.
	for len(lines) < rows+2 {
		lines = append(lines, "")
	}

	if withTotals {
		lines = append(lines, m.rule(width))
		lines = append(lines, m.totalsRow(s, width))
	}

	return lines
}

// nameColWidth is what remains of the width for the file names.
func nameColWidth(width int) int {
	return max(width-2-3*numColWidth-detailsColWidth-2, 10)
}

// tableTitles draws the column titles; with staging, a narrow column
// for the staging marks comes first.
func (m *Model) tableTitles(width int, staging bool) string {
	var b strings.Builder

	b.WriteString("  ")
	if staging {
		b.WriteString("  ")
	}
	b.WriteString(padLeft("Removed", numColWidth))
	b.WriteString(padLeft("Added", numColWidth))
	b.WriteString(padLeft("Modified", numColWidth))
	b.WriteString("  ")
	b.WriteString(padRight("Details", detailsColWidth))
	b.WriteString("Filename")

	return m.pal.Style(m.pal.Muted, m.pal.Bg, true).Render(padTo(b.String(), width))
}

// rule draws a horizontal separator.
func (m *Model) rule(width int) string {
	return m.pal.Paint(strings.Repeat("─", width), m.pal.Border)
}

// stageMark shows how much of a file is staged: ● all, ◐ part, ○ none.
func (m *Model) stageMark(f *review.File) (string, color.Color) {
	switch m.status.Files[f.Change.Path()] {
	case git.Staged:
		return "● ", m.pal.AddedFg
	case git.Partly:
		return "◐ ", m.pal.ModifiedFg
	}
	return "○ ", m.pal.Muted
}

// fileRow draws one file of the table, after its staging mark when
// staging is set.
func (m *Model) fileRow(f *review.File, staging, selected, focused bool, width int) string {
	var highlight bool
	var bg color.Color
	var marker string
	var counts [3]string
	var b strings.Builder

	highlight = selected && focused
	bg = m.pal.Bg
	marker = "  "
	if selected {
		marker = "▸ "
	}
	if highlight {
		bg = m.pal.Selection
	}

	counts = countTexts(f)

	// Counts first, then the kind of change, then the name, so the eye
	// stays on the left side of the screen.
	b.WriteString(m.pal.Style(m.pal.Header, bg, true).Render(marker))
	if staging {
		mark, fg := m.stageMark(f)
		b.WriteString(m.pal.Style(fg, bg, false).Render(mark))
	}
	b.WriteString(m.pal.Style(m.pal.RemovedFg, bg, false).Render(padLeft(counts[0], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.AddedFg, bg, false).Render(padLeft(counts[1], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, bg, false).Render(padLeft(counts[2], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render("  " + padRight(details(f, highlight), detailsColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, bg, highlight).Render(displayName(f.Change, nameColWidth(width))))

	return m.padLine(b.String(), bg, width)
}

// padLine cuts a line to width and fills the rest with a background color.
func (m *Model) padLine(line string, bg color.Color, width int) string {
	var w int

	line = ansi.Truncate(line, width, "…")
	w = ansi.StringWidth(line)
	if w >= width {
		return line
	}
	return line + m.pal.Style(nil, bg, false).Render(strings.Repeat(" ", width-w))
}

// totalsRow sums the counts of the visible files.
func (m *Model) totalsRow(s *changeSet, width int) string {
	var total diff.Counts
	var label string
	var b strings.Builder

	total = s.totals()

	label = "Total: " + fileCount(len(s.list.visible))
	if s.list.hidden > 0 {
		label += fmt.Sprintf(" (%d hidden: only ignored changes)", s.list.hidden)
	}

	// Totals line up under the count columns; the label takes the
	// details and filename columns.
	b.WriteString(m.pal.Paint("  ", nil))
	b.WriteString(m.pal.Style(m.pal.RemovedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Removed), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.AddedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Added), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Modified), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, m.pal.Bg, true).Render("  " + label))

	return padTo(b.String(), width)
}

// countTexts formats the three counts; binary files have none.
func countTexts(f *review.File) [3]string {
	if f.Binary {
		return [3]string{"-", "-", "-"}
	}
	return [3]string{
		fmt.Sprint(f.Counts.Removed),
		fmt.Sprint(f.Counts.Added),
		fmt.Sprint(f.Counts.Modified),
	}
}

// details describes the kind of change; the highlighted row shows the
// key that opens it.
func details(f *review.File, highlight bool) string {
	var text string

	text = statusText(f.Change.Status)
	if f.Binary {
		text = "binary"
	}
	if highlight {
		text += " ⏎"
	}

	return strings.TrimSpace(text)
}

// statusText names a git status.
func statusText(s git.Status) string {
	switch s {
	case git.Added, git.Untracked:
		return "new file"
	case git.Deleted:
		return "deleted"
	case git.Renamed:
		return "renamed"
	case git.Copied:
		return "copied"
	case git.TypeChg:
		return "type changed"
	}
	return ""
}

// displayName shows "old → new" for renames, cut from the left to fit
// so that the file name stays visible.
func displayName(c git.Change, width int) string {
	var name string
	var runes []rune

	name = c.Path()
	if (c.Status == git.Renamed || c.Status == git.Copied) && c.OldPath != c.NewPath {
		name = c.OldPath + " → " + c.NewPath
	}

	runes = []rune(name)
	if len(runes) > width-1 {
		return "…" + string(runes[len(runes)-(width-2):])
	}
	return name
}
