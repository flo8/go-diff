// Package tui is the interactive interface: the file summary, the diff
// viewer and the help screen.
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/render"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/theme"
)

// Options configure a new Model.
type Options struct {
	Files   []*review.File // already computed with Filters
	Range   string         // description of what is compared
	Filters semantic.Filters
	Themes  []theme.Theme
	Theme   string // theme name, or "" to pick dark or light from the terminal
}

// Model is the Bubble Tea model of the whole application.
type Model struct {
	files     []*review.File
	rangeName string
	filters   semantic.Filters

	themes    []theme.Theme
	themeIdx  int
	autoTheme bool
	pal       *render.Palette

	list   fileList
	viewer *viewer // nil on the summary screen
	help   bool

	width  int
	height int
}

// New creates the model. Files must already be computed with opt.Filters.
func New(opt Options) *Model {
	var m *Model

	m = &Model{
		files:     opt.Files,
		rangeName: opt.Range,
		filters:   opt.Filters,
		themes:    opt.Themes,
		autoTheme: opt.Theme == "",
	}

	m.themeIdx = themeIndex(m.themes, opt.Theme)
	m.pal = render.NewPalette(m.themes[m.themeIdx])
	m.list.refresh(m.files)

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
	default:
		m.screenKey(key)
	}

	return nil
}

// screenKey sends a key to the summary or the viewer.
func (m *Model) screenKey(key string) {
	if m.viewer == nil {
		m.summaryKey(key)
		return
	}
	m.viewerKey(key)
}

// toggleFilter flips one filter and recomputes every file.
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

	review.ComputeAll(m.files, m.filters)
	m.list.refresh(m.files)

	// The open file keeps its place in the new alignment.
	if m.viewer != nil {
		m.viewer.reload(m.height)
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
	case m.viewer != nil:
		content = m.viewerView()
	default:
		content = m.summaryView()
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
