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

// Column widths of the summary table.
const (
	numColWidth     = 9
	detailsColWidth = 14
	summaryChrome   = 6 // header, column titles, two rules, totals, footer
)

// fileList is the state of the summary table.
type fileList struct {
	visible       []*review.File // files with meaningful changes, sorted
	hidden        int            // files changed only in ignored ways
	sortByChanges bool
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

	// Find the previously selected file again.
	l.selected = 0
	for i, f := range l.visible {
		if f == keep {
			l.selected = i
		}
	}
}

// sort orders by path, or by number of changes (most first).
func (l *fileList) sort() {
	if l.sortByChanges {
		sort.SliceStable(l.visible, l.byChanges)
		return
	}
	sort.SliceStable(l.visible, l.byPath)
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

// summaryKey handles keys on the summary screen.
func (m *Model) summaryKey(key string) {
	var page int

	page = max(m.height-summaryChrome, 1)

	switch key {
	case "up", "k":
		m.list.move(-1)
	case "down", "j":
		m.list.move(1)
	case "pgup", "ctrl+b":
		m.list.move(-page)
	case "pgdown", "ctrl+f", "space":
		m.list.move(page)
	case "home", "g":
		m.list.move(-len(m.list.visible))
	case "end", "G":
		m.list.move(len(m.list.visible))
	case "o":
		m.list.sortByChanges = !m.list.sortByChanges
		m.list.refresh(m.files)
	case "enter", "l", "right":
		m.openSelected()
	}
}

// openSelected opens the diff viewer on the selected file.
func (m *Model) openSelected() {
	if len(m.list.visible) == 0 {
		return
	}
	m.viewer = newViewer(m.list.visible[m.list.selected], m.height)
}

// summaryView draws the header, the file table, the totals and the footer.
func (m *Model) summaryView() string {
	var lines []string
	var listHeight int

	listHeight = max(m.height-summaryChrome, 1)
	m.scrollList(listHeight)

	lines = append(lines, m.headerLine(m.rangeName))
	lines = append(lines, m.tableTitles())
	lines = append(lines, m.rule())

	for i := m.list.top; i < len(m.list.visible) && i < m.list.top+listHeight; i++ {
		lines = append(lines, m.fileRow(m.list.visible[i], i == m.list.selected))
	}

	// An empty list says why.
	if len(m.list.visible) == 0 {
		lines = append(lines, m.pal.Paint("  No meaningful changes.", m.pal.Muted))
	}

	// Keep the totals and footer at the bottom of the screen.
	for len(lines) < listHeight+3 {
		lines = append(lines, "")
	}

	lines = append(lines, m.rule())
	lines = append(lines, m.totalsRow())
	lines = append(lines, m.summaryFooter())

	return strings.Join(lines, "\n")
}

// scrollList keeps the selected row on screen.
func (m *Model) scrollList(height int) {
	if m.list.selected < m.list.top {
		m.list.top = m.list.selected
	}
	if m.list.selected >= m.list.top+height {
		m.list.top = m.list.selected - height + 1
	}
}

// nameColWidth is what remains of the width for the file names.
func (m *Model) nameColWidth() int {
	return max(m.width-2-3*numColWidth-detailsColWidth-2, 10)
}

// headerLine shows the app name and range on the left and the filter
// switches on the right.
func (m *Model) headerLine(title string) string {
	var left string

	var badges string

	left = m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(" godiff ") +
		m.pal.Paint(title, m.pal.Fg)

	// Drop the filter names when the line is too narrow for them.
	badges = m.filterBadges(false) + " "
	if ansi.StringWidth(left)+ansi.StringWidth(badges)+2 > m.width {
		badges = m.filterBadges(true) + " "
	}

	return m.spread(left, badges)
}

// filterBadges shows each filter's key and whether it is on, as
// "c comments ✓", or "c✓" when compact.
func (m *Model) filterBadges(compact bool) string {
	var parts []string

	parts = append(parts, m.badge("c", "comments", m.filters.Comments, compact))
	parts = append(parts, m.badge("w", "whitespace", m.filters.Whitespace, compact))
	parts = append(parts, m.badge("f", "formatting", m.filters.Formatting, compact))
	parts = append(parts, m.badge("i", "imports", m.filters.Imports, compact))

	return m.pal.Paint("ignore: ", m.pal.Muted) + strings.Join(parts, m.pal.Paint("  ", nil))
}

// badge draws one filter with its key highlighted.
func (m *Model) badge(key, name string, on, compact bool) string {
	var mark string
	var fg color.Color
	var label string

	mark = "✗"
	fg = m.pal.Muted
	if on {
		mark = "✓"
		fg = m.pal.Fg
	}

	label = " " + name + " " + mark
	if compact {
		label = mark
	}

	return m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(key) + m.pal.Paint(label, fg)
}

// tableTitles draws the column titles.
func (m *Model) tableTitles() string {
	var b strings.Builder

	b.WriteString("  ")
	b.WriteString(padLeft("Removed", numColWidth))
	b.WriteString(padLeft("Added", numColWidth))
	b.WriteString(padLeft("Modified", numColWidth))
	b.WriteString("  ")
	b.WriteString(padRight("Details", detailsColWidth))
	b.WriteString("Filename")

	return m.pal.Style(m.pal.Muted, m.pal.Bg, true).Render(m.fit(b.String()))
}

// rule draws a horizontal separator.
func (m *Model) rule() string {
	return m.pal.Paint(strings.Repeat("─", m.width), m.pal.Border)
}

// fileRow draws one file of the table.
func (m *Model) fileRow(f *review.File, selected bool) string {
	var bg color.Color
	var marker string
	var counts [3]string
	var b strings.Builder

	bg = m.pal.Bg
	marker = "  "
	if selected {
		bg = m.pal.Selection
		marker = "▸ "
	}

	counts = countTexts(f)

	// Counts first, then the kind of change, then the name, so the eye
	// stays on the left side of the screen.
	b.WriteString(m.pal.Style(m.pal.Header, bg, true).Render(marker))
	b.WriteString(m.pal.Style(m.pal.RemovedFg, bg, false).Render(padLeft(counts[0], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.AddedFg, bg, false).Render(padLeft(counts[1], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, bg, false).Render(padLeft(counts[2], numColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render("  " + padRight(details(f, selected), detailsColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, bg, selected).Render(displayName(f.Change, m.nameColWidth())))

	return m.padLine(b.String(), bg)
}

// padLine fills the rest of a line with a background color.
func (m *Model) padLine(line string, bg color.Color) string {
	var w int

	line = m.fit(line)
	w = ansi.StringWidth(line)
	if w >= m.width {
		return line
	}
	return line + m.pal.Style(nil, bg, false).Render(strings.Repeat(" ", m.width-w))
}

// totalsRow sums the counts of the visible files.
func (m *Model) totalsRow() string {
	var total diff.Counts
	var label string
	var b strings.Builder

	for _, f := range m.list.visible {
		total.Removed += f.Counts.Removed
		total.Added += f.Counts.Added
		total.Modified += f.Counts.Modified
	}

	label = fmt.Sprintf("Total: %d files", len(m.list.visible))
	if m.list.hidden > 0 {
		label += fmt.Sprintf(" (%d hidden: only ignored changes)", m.list.hidden)
	}

	// Totals line up under the count columns; the label takes the
	// details and filename columns.
	b.WriteString(m.pal.Paint("  ", nil))
	b.WriteString(m.pal.Style(m.pal.RemovedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Removed), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.AddedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Added), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, m.pal.Bg, true).Render(padLeft(fmt.Sprint(total.Modified), numColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, m.pal.Bg, true).Render("  " + label))

	return m.fit(b.String())
}

// summaryFooter lists the main keys.
func (m *Model) summaryFooter() string {
	var sortName string

	sortName = "name"
	if m.list.sortByChanges {
		sortName = "changes"
	}

	return m.keyHints([][2]string{
		{"↑↓", "select"}, {"⏎", "open"}, {"o", "sort: " + sortName},
		{"t", "theme: " + m.pal.Theme.Name}, {"?", "help"}, {"q", "quit"},
	})
}

// keyHints draws "key action" pairs.
func (m *Model) keyHints(hints [][2]string) string {
	var parts []string

	for _, h := range hints {
		parts = append(parts, m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(h[0])+m.pal.Paint(" "+h[1], m.pal.Muted))
	}

	return m.fit(" " + strings.Join(parts, m.pal.Paint("   ", nil)))
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

// details describes the kind of change; the selected row shows the key
// that opens it.
func details(f *review.File, selected bool) string {
	var text string

	text = statusText(f.Change.Status)
	if f.Binary {
		text = "binary"
	}
	if selected {
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

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(width-ansi.StringWidth(s), 0)) + s
}
