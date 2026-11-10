// Package tui is the interactive interface: the file table with the
// history panel under it, the diff viewer and the help screen.
package tui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/render"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/theme"
)

// focusArea is the panel that receives the arrow keys.
type focusArea int

const (
	focusFiles focusArea = iota
	focusHistory
)

// Options configure a new Model.
type Options struct {
	Repo    *git.Repo
	Files   []*review.File // already computed with Filters
	Range   string         // description of what is compared
	Filters semantic.Filters
	Themes  []theme.Theme
	Theme   string // theme name, or "" to pick dark or light from the terminal
}

// Model is the Bubble Tea model of the whole application.
type Model struct {
	repo    *git.Repo
	filters semantic.Filters

	themes    []theme.Theme
	themeIdx  int
	autoTheme bool
	pal       *render.Palette

	current     *changeSet // the comparison given on the command line
	hist        history
	files       pane // the selected history entry's files and open viewer
	focus       focusArea
	showHistory bool
	help        bool

	width  int
	height int
}

// New creates the model. Files must already be computed with opt.Filters.
func New(opt Options) *Model {
	var m *Model

	m = &Model{
		repo:        opt.Repo,
		filters:     opt.Filters,
		themes:      opt.Themes,
		autoTheme:   opt.Theme == "",
		showHistory: true,
	}

	m.themeIdx = themeIndex(m.themes, opt.Theme)
	m.pal = render.NewPalette(m.themes[m.themeIdx])

	// "current" is selected first; commits load when selected.
	m.current = newChangeSet(opt.Files, opt.Range, opt.Filters)
	m.files = pane{set: m.current}
	m.hist.cache = map[string]*changeSet{}
	m.hist.loadErr = map[string]error{}
	if m.repo != nil {
		m.hist.commits, m.hist.err = m.repo.Log(logLimit)
	}

	return m
}

// Init asks the terminal for its background color, to choose between
// the dark and light themes when none was configured.
func (m *Model) Init() tea.Cmd {
	if m.autoTheme {
		return tea.RequestBackgroundColor
	}
	return nil
}

// Update handles one event.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// The size decides which history rows are visible: load their counts.
		return m, m.nextLoad()
	case tea.BackgroundColorMsg:
		if m.autoTheme && !msg.IsDark() {
			m.setTheme(themeIndex(m.themes, "light"))
		}
	case loadTick:
		return m, m.startLoad(msg)
	case commitLoaded:
		return m, m.finishLoad(msg)
	case tea.KeyPressMsg:
		return m, m.handleKey(msg.String())
	}

	return m, nil
}

// handleKey runs the keys that work everywhere, then the panel's own.
func (m *Model) handleKey(key string) tea.Cmd {

	// Any key closes the help screen.
	if m.help {
		m.help = false
		if key == "q" || key == "ctrl+c" {
			return tea.Quit
		}
		return nil
	}

	switch key {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		m.help = true
	case "t":
		m.autoTheme = false
		m.setTheme((m.themeIdx + 1) % len(m.themes))
	case "c", "w", "f", "i":
		m.toggleFilter(key)
	case "tab":
		m.toggleFocus()
	case "L":
		m.toggleHistory()
	default:
		return m.screenKey(key)
	}

	return nil
}

// toggleFocus moves the focus between the file table and the history,
// showing the history if it was hidden. It does nothing in the viewer.
func (m *Model) toggleFocus() {
	if m.files.viewer != nil {
		return
	}

	if m.focus == focusHistory {
		m.focus = focusFiles
		return
	}

	m.showHistory = true
	m.focus = focusHistory
}

// toggleHistory shows or hides the history panel.
func (m *Model) toggleHistory() {
	m.showHistory = !m.showHistory
	if !m.showHistory {
		m.focus = focusFiles
	}
}

// screenKey sends a key to the open viewer or the focused panel.
func (m *Model) screenKey(key string) tea.Cmd {
	switch {
	case m.files.viewer != nil:
		m.viewerKey(&m.files, key)
	case m.focus == focusHistory:
		return m.historyKey(key)
	case m.files.set != nil:
		m.fileListKey(&m.files, key)
	}

	return nil
}

// toggleFilter flips one filter and recomputes what is on screen. Other
// loaded commits are recomputed when they are shown again.
func (m *Model) toggleFilter(key string) {
	switch key {
	case "c":
		m.filters.Comments = !m.filters.Comments
	case "w":
		m.filters.Whitespace = !m.filters.Whitespace
	case "f":
		m.filters.Formatting = !m.filters.Formatting
	case "i":
		m.filters.Imports = !m.filters.Imports
	}

	// The history rows on screen show totals: keep them up to date.
	// Commits off screen are recomputed when they come into view.
	m.current.recompute(m.filters)
	for _, hash := range m.visibleCommits() {
		if set := m.hist.cache[hash]; set != nil {
			set.recompute(m.filters)
		}
	}

	if m.files.set != nil {
		m.files.set.recompute(m.filters)
	}
	if m.files.viewer != nil {
		m.files.viewer.reload(m.height)
	}
}

// setTheme switches to theme i.
func (m *Model) setTheme(i int) {
	m.themeIdx = i
	m.pal = render.NewPalette(m.themes[i])
}

// View draws the current screen.
func (m *Model) View() tea.View {
	var content string
	var v tea.View

	switch {
	case m.width == 0 || m.height == 0:
		content = ""
	case m.help:
		content = m.helpView()
	case m.files.viewer != nil:
		content = m.viewerView(&m.files)
	default:
		content = m.mainView()
	}

	v = tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "godiff"

	// The theme background becomes the terminal background, so every
	// unstyled cell gets it too.
	v.BackgroundColor = m.pal.Bg
	v.ForegroundColor = m.pal.Fg

	return v
}

// mainView draws the header, the file table, the history panel (when
// shown) and the footer.
func (m *Model) mainView() string {
	var tableHeight int
	var lines []string

	tableHeight = m.height - 2
	if m.showHistory {
		tableHeight -= m.historyHeight()
	}

	lines = append(lines, m.headerLine(m.selectedTitle()))
	lines = append(lines, m.filesPanel(max(tableHeight, 1))...)
	if m.showHistory {
		lines = append(lines, m.historyPanel(m.historyHeight())...)
	}
	lines = append(lines, m.mainFooter())

	return strings.Join(lines, "\n")
}

// selectedTitle names the selected history entry in the header.
func (m *Model) selectedTitle() string {
	commit, isCommit := m.hist.selectedCommit()
	if !isCommit {
		return m.current.rangeName
	}
	return commit.Short + " " + commit.Subject
}

// filesPanel draws the selected entry's file table in height lines, or
// why there is none yet.
func (m *Model) filesPanel(height int) []string {
	var lines []string

	commit, isCommit := m.hist.selectedCommit()
	switch {
	case m.files.set != nil:
		return m.fileTable(m.files.set, m.width, height, m.focus == focusFiles, !m.showHistory)
	case isCommit && m.hist.loadErr[commit.Hash] != nil:
		lines = append(lines, m.pal.Paint("  "+m.hist.loadErr[commit.Hash].Error(), m.pal.RemovedFg))
	default:
		lines = append(lines, m.pal.Paint("  Loading…", m.pal.Muted))
	}

	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// mainFooter lists the keys of the focused panel.
func (m *Model) mainFooter() string {
	var historyKey [2]string

	historyKey = [2]string{"L", "hide history"}
	if !m.showHistory {
		historyKey = [2]string{"L", "show history"}
	}

	if m.focus == focusHistory {
		return m.keyHints([][2]string{
			{"↑↓", "select entry"}, {"⏎/tab", "files"},
			historyKey, {"?", "help"}, {"q", "quit"},
		})
	}

	return m.keyHints([][2]string{
		{"↑↓", "select file"}, {"⏎", "open"}, {"o", "sort: " + m.sortName()},
		{"tab", "history"}, historyKey, {"t", "theme: " + m.pal.Theme.Name}, {"?", "help"}, {"q", "quit"},
	})
}

// sortName names the sort order of the shown file table.
func (m *Model) sortName() string {
	if m.files.set != nil && m.files.set.list.sortByChanges {
		return "changes"
	}
	return "name"
}

// headerLine shows the app name and a title on the left and the filter
// switches on the right.
func (m *Model) headerLine(title string) string {
	var left string
	var badges string

	left = m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(" godiff ") +
		m.pal.Paint(" "+title, m.pal.Fg)

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

// keyHints draws "key action" pairs.
func (m *Model) keyHints(hints [][2]string) string {
	var parts []string

	for _, h := range hints {
		parts = append(parts, m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(h[0])+m.pal.Paint(" "+h[1], m.pal.Muted))
	}

	return m.fit(" " + strings.Join(parts, m.pal.Paint("   ", nil)))
}

// fit truncates a styled line to the screen width.
func (m *Model) fit(line string) string {
	return ansi.Truncate(line, m.width, "…")
}

// spread puts left and right on one line separated by spaces.
func (m *Model) spread(left, right string) string {
	var gap int

	gap = m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return m.fit(left + " " + right)
	}
	return left + strings.Repeat(" ", gap) + right
}

// themeIndex returns the index of a theme by name, or 0.
func themeIndex(themes []theme.Theme, name string) int {
	for i, t := range themes {
		if t.Name == name {
			return i
		}
	}
	return 0
}
