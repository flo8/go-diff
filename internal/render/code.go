package render

import (
	"image/color"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/syntax"
)

// tabWidth is how many columns a tab stop spans.
const tabWidth = 4

// cell describes how one line of code is drawn.
type cell struct {
	text   string
	syntax []syntax.Span // colors, sorted by Start
	words  []diff.Span   // changed words, sorted by Start
	width  int           // columns to fill
	xoff   int           // columns scrolled off to the left
	bg     color.Color   // line background
	wordBg color.Color   // background of changed words
}

// codeWriter collects runs of text that share the same colors and
// renders each run once.
type codeWriter struct {
	p    *Palette
	out  strings.Builder
	run  strings.Builder
	fg   color.Color
	bg   color.Color
	used int // columns written
}

// write adds text that takes w columns, in the given colors.
func (cw *codeWriter) write(text string, w int, fg, bg color.Color) {
	if fg != cw.fg || bg != cw.bg {
		cw.flush()
		cw.fg = fg
		cw.bg = bg
	}
	cw.run.WriteString(text)
	cw.used += w
}

// flush renders the pending run.
func (cw *codeWriter) flush() {
	if cw.run.Len() == 0 {
		return
	}
	cw.out.WriteString(cw.p.Style(cw.fg, cw.bg, false).Render(cw.run.String()))
	cw.run.Reset()
}

// code draws one line of code: syntax colors as foreground, the line
// background, and a stronger background on changed words. Tabs are
// expanded, and the text is clipped and padded to exactly c.width columns.
func (p *Palette) code(c cell) string {
	var cw codeWriter
	var col int
	var si, wi int

	cw = codeWriter{p: p}

	for b, r := range c.text {

		// Move the syntax and word cursors up to this byte.
		for si < len(c.syntax) && c.syntax[si].End <= b {
			si++
		}
		for wi < len(c.words) && c.words[wi].End <= b {
			wi++
		}

		fg := p.Fg
		if si < len(c.syntax) && c.syntax[si].Start <= b {
			fg = p.Syntax[c.syntax[si].Class]
		}
		bg := c.bg
		if wi < len(c.words) && c.words[wi].Start <= b {
			bg = or(c.wordBg, c.bg)
		}

		// What the rune looks like on screen, and how wide it is.
		glyph, w := glyphOf(r, col)

		// Clip against the horizontal scroll window [xoff, xoff+width).
		if col >= c.xoff+c.width {
			break
		}
		if col+w <= c.xoff {
			col += w
			continue
		}
		if col < c.xoff || col+w > c.xoff+c.width {
			glyph = strings.Repeat(" ", visible(col, w, c.xoff, c.width))
		}

		cw.write(glyph, min(w, len(glyph)), fg, bg)
		col += w
	}

	// Pad the rest of the cell with the line background.
	if cw.used < c.width {
		cw.write(strings.Repeat(" ", c.width-cw.used), c.width-cw.used, p.Fg, c.bg)
	}

	cw.flush()
	return cw.out.String()
}

// glyphOf returns the text drawn for r at column col and its width.
// Tabs become spaces up to the next tab stop and control characters are
// made visible.
func glyphOf(r rune, col int) (string, int) {
	var n int

	switch {
	case r == '\t':
		n = tabWidth - col%tabWidth
		return strings.Repeat(" ", n), n
	case r == '\r':
		return "", 0
	case r < 0x20 || r == 0x7f:
		return "·", 1
	}

	return string(r), runewidth.RuneWidth(r)
}

// visible returns how many of the w columns starting at col fall
// inside the window [xoff, xoff+width).
func visible(col, w, xoff, width int) int {
	var start, end int

	start = max(col, xoff)
	end = min(col+w, xoff+width)
	return max(end-start, 0)
}
