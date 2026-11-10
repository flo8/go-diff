// Package tui is the interactive interface: the changes tab, the log
// tab, the diff viewer and the help screen.
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

// Tabs, switched with "1", "2" and "tab".
const (
	tabChanges = iota
	tabLog
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

	tab     int
	changes pane     // the comparison given on the command line
	log     logState // the commit history
	help    bool

	width  int
	height int
}

// New creates the model. Files must already be computed with opt.Filters.
func New(opt Options) *Model {
	var m *Model

	m = &Model{
		repo:      opt.Repo,
		filters:   opt.Filters,
		themes:    opt.Themes,
		autoTheme: opt.Theme == "",
	}

	m.themeIdx = themeIndex(m.themes, opt.Theme)
	m.pal = render.NewPalette(m.themes[m.themeIdx])
	m.changes.set = newChangeSet(opt.Files, opt.Range, opt.Filters)

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

// handleKey runs the keys that work everywhere, then the screen's own.
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
	case "1":
		return m.switchTab(tabChanges)
	case "2":
		return m.switchTab(tabLog)
	case "tab":
		return m.switchTab(1 - m.tab)
	default:
		return m.screenKey(key)
	}

	return nil
}

// switchTab shows a tab where it was left. The log is read the first
// time it is shown.
func (m *Model) switchTab(tab int) tea.Cmd {
	m.tab = tab
	if tab == tabLog {
		return m.openLog()
	}
	return nil
}

// activePane is the file table and viewer of the current tab.
func (m *Model) activePane() *pane {
	if m.tab == tabLog {
		return &m.log.files
	}
	return &m.changes
}

// screenKey sends a key to the open viewer, or to the tab.
func (m *Model) screenKey(key string) tea.Cmd {
	var p *pane

	p = m.activePane()
	switch {
	case p.viewer != nil:
		m.viewerKey(p, key)
	case m.tab == tabLog:
		return m.logKey(key)
	default:
		m.fileListKey(p, key)
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

	m.refreshPane(&m.changes)
	m.refreshPane(&m.log.files)
}

// refreshPane recomputes a pane's files; an open file keeps its place.
func (m *Model) refreshPane(p *pane) {
	if p.set == nil {
		return
	}

	p.set.recompute(m.filters)
	if p.viewer != nil {
		p.viewer.reload(m.height)
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
	var p *pane
	var v tea.View

	p = m.activePane()
	switch {
	case m.width == 0 || m.height == 0:
		content = ""
	case m.help:
		content = m.helpView()
	case p.viewer != nil:
		content = m.viewerView(p)
	case m.tab == tabLog:
		content = m.logView()
	default:
		content = m.changesView()
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

// headerLine shows the app name, the tabs and a title on the left and
// the filter switches on the right.
func (m *Model) headerLine(title string) string {
	var left string
	var badges string

	left = m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(" godiff ") +
		m.tabLabels() + m.pal.Paint("  "+title, m.pal.Fg)

	// Drop the filter names when the line is too narrow for them.
	badges = m.filterBadges(false) + " "
	if ansi.StringWidth(left)+ansi.StringWidth(badges)+2 > m.width {
		badges = m.filterBadges(true) + " "
	}

	return m.spread(left, badges)
}

// tabLabels draws "1 Changes  2 Log" with the current tab highlighted.
func (m *Model) tabLabels() string {
	return m.tabLabel("1 Changes", m.tab == tabChanges) + m.tabLabel("2 Log", m.tab == tabLog)
}

func (m *Model) tabLabel(text string, active bool) string {
	if active {
		return m.pal.Style(m.pal.Header, m.pal.Selection, true).Render(" " + text + " ")
	}
	return m.pal.Paint(" "+text+" ", m.pal.Muted)
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
