package tui

import (
	"strings"
)

// helpSections lists every key, by screen.
var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"Everywhere", [][2]string{
		{"F c", "ignore comment changes (on/off)"},
		{"F w", "ignore whitespace changes (on/off)"},
		{"F f", "ignore formatting changes: spacing, trailing commas (on/off)"},
		{"F i", "ignore import changes, Go only (on/off)"},
		{"t", "next theme"},
		{"?", "show this help"},
		{"q", "quit"},
	}},
	{"Git", [][2]string{
		{"space", "stage / unstage the selected file (\"current\" only)"},
		{"a", "stage everything / unstage everything"},
		{"c", "commit the staged files, with a one-line message"},
		{"C", "commit with your editor ($EDITOR) for a full message"},
		{"p", "pull: fast-forward only, never merges"},
		{"P", "push: sets the upstream the first time, never forces"},
	}},
	{"Panels", [][2]string{
		{"tab", "move between the file table and the history"},
		{"L", "show / hide the history panel"},
	}},
	{"Mouse", [][2]string{
		{"click", "select a file or a history entry"},
		{"double-click", "open a file"},
		{"click a package", "show only its files; click again or Esc for all"},
		{"wheel", "scroll the panel under the mouse, or the diff"},
		{"Shift+drag", "select text (in most terminals)"},
	}},
	{"History", [][2]string{
		{"↑ ↓  j k", "select an entry; its files show in the table above"},
		{"", "\"current\" is the comparison from the command line"},
		{"Enter  Esc", "back to the file table"},
	}},
	{"File table", [][2]string{
		{"↑ ↓  j k", "select a file"},
		{"PgUp PgDn  g G", "page up / down, first / last"},
		{"Enter", "open the selected file"},
		{"Esc", "show all packages after clicking one"},
		{"o", "group by go files, tests and other files (default) or not"},
	}},
	{"Diff viewer", [][2]string{
		{"n  N", "next / previous change"},
		{"↑ ↓  j k", "scroll one line"},
		{"PgUp PgDn  ^U ^D", "scroll a page / half a page"},
		{"g  G", "top / bottom"},
		{"← →  h l", "scroll left / right"},
		{"s", "side-by-side or unified view"},
		{"[  ]", "previous / next file"},
		{"Esc", "back to the summary"},
	}},
}

// helpView draws the list of keys.
func (m *Model) helpView() string {
	var lines []string

	lines = append(lines, m.headerLine("help"))

	for _, section := range helpSections {
		lines = append(lines, "")
		lines = append(lines, m.pal.Style(m.pal.Header, m.pal.Bg, true).Render("  "+section.title))
		for _, k := range section.keys {
			key := m.pal.Style(m.pal.Fg, m.pal.Bg, true).Render("    " + padRight(k[0], 20))
			lines = append(lines, m.fit(key+m.pal.Paint(k[1], m.pal.Muted)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, m.pal.Paint("  Press any key to close.", m.pal.Muted))

	return strings.Join(lines, "\n")
}
