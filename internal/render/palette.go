// Package render draws file diffs as lines of styled terminal text,
// side by side or unified. It does not handle input; package tui does.
package render

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/flo8/go-diff/internal/syntax"
	"github.com/flo8/go-diff/internal/theme"
)

// Palette is a theme turned into terminal colors, plus a cache of styles
// so each color pair is built once.
type Palette struct {
	Theme theme.Theme

	Bg, Fg, Muted, Header, Border  color.Color
	LineNumber, Selection, Cursor  color.Color
	Filler                         color.Color
	AddedBg, RemovedBg             color.Color
	AddedWord, RemovedWord         color.Color
	AddedFg, RemovedFg, ModifiedFg color.Color
	Syntax                         [syntax.Operator + 1]color.Color

	styles map[styleKey]lipgloss.Style
}

// styleKey identifies a cached style.
type styleKey struct {
	fg   color.Color
	bg   color.Color
	bold bool
}

// NewPalette converts a theme's color strings.
func NewPalette(t theme.Theme) *Palette {
	var p *Palette

	p = &Palette{Theme: t, styles: map[styleKey]lipgloss.Style{}}

	p.Bg = toColor(t.Background)
	p.Fg = toColor(t.Foreground)
	p.Muted = toColor(t.Muted)
	p.Header = toColor(t.Header)
	p.Border = toColor(t.Border)
	p.LineNumber = toColor(t.LineNumber)
	p.Selection = toColor(t.Selection)
	p.Cursor = toColor(t.Cursor)
	p.Filler = toColor(t.Filler)
	p.AddedBg = toColor(t.AddedBg)
	p.RemovedBg = toColor(t.RemovedBg)
	p.AddedWord = toColor(t.AddedWord)
	p.RemovedWord = toColor(t.RemovedWord)
	p.AddedFg = toColor(t.AddedFg)
	p.RemovedFg = toColor(t.RemovedFg)
	p.ModifiedFg = toColor(t.ModifiedFg)

	// Plain text uses the foreground color.
	p.Syntax[syntax.Plain] = p.Fg
	p.Syntax[syntax.Keyword] = toColor(t.Syntax.Keyword)
	p.Syntax[syntax.Type] = toColor(t.Syntax.Type)
	p.Syntax[syntax.Function] = toColor(t.Syntax.Function)
	p.Syntax[syntax.String] = toColor(t.Syntax.String)
	p.Syntax[syntax.Number] = toColor(t.Syntax.Number)
	p.Syntax[syntax.Comment] = toColor(t.Syntax.Comment)
	p.Syntax[syntax.Operator] = toColor(t.Syntax.Operator)

	return p
}

// Style returns a style with the given colors; nil means terminal default.
func (p *Palette) Style(fg, bg color.Color, bold bool) lipgloss.Style {
	var key styleKey
	var s lipgloss.Style

	key = styleKey{fg: fg, bg: bg, bold: bold}
	s, ok := p.styles[key]
	if ok {
		return s
	}

	s = lipgloss.NewStyle().Bold(bold)
	if fg != nil {
		s = s.Foreground(fg)
	}
	if bg != nil {
		s = s.Background(bg)
	}

	p.styles[key] = s
	return s
}

// Paint renders text with a foreground on the theme background.
func (p *Palette) Paint(text string, fg color.Color) string {
	return p.Style(fg, p.Bg, false).Render(text)
}

// toColor parses a theme color string; "" is the terminal default.
func toColor(s string) color.Color {
	if s == "" {
		return nil
	}
	return lipgloss.Color(s)
}

// or returns c, or fallback when c is nil.
func or(c, fallback color.Color) color.Color {
	if c == nil {
		return fallback
	}
	return c
}
