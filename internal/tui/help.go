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
		{"c", "ignore comment changes (on/off)"},
		{"w", "ignore whitespace changes (on/off)"},
		{"f", "ignore formatting changes: spacing, trailing commas (on/off)"},
		{"i", "ignore import changes, Go only (on/off)"},
		{"t", "next theme"},
		{"?", "show this help"},
		{"q", "quit"},
	}},
	{"File summary", [][2]string{
		{"↑ ↓  j k", "select a file"},
		{"PgUp PgDn  g G", "page up / down, first / last"},
		{"Enter", "open the selected file"},
		{"o", "sort by name or by number of changes"},
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
