package render

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/syntax"
)

// Viewport is the visible part of a document.
type Viewport struct {
	Top     int   // first display line shown
	Height  int   // number of screen lines
	Width   int   // number of screen columns
	XOffset int   // columns scrolled to the right
	Current Block // the change the cursor is on, drawn with Cursor numbers
}

// version is one side of the file as the drawing code needs it.
type version struct {
	lines  []string
	syntax [][]syntax.Span
}

// Draw renders the visible display lines, one string per screen line.
// Lines past the end of the document are left out.
func Draw(doc *Doc, lines []DisplayLine, mode Mode, vp Viewport, p *Palette) []string {
	var out []string
	var numWidth int

	numWidth = len(fmt.Sprint(max(len(doc.Old), len(doc.New), 99)))
	out = make([]string, 0, vp.Height)

	for i := vp.Top; i < len(lines) && i < vp.Top+vp.Height; i++ {
		current := i >= vp.Current.Start && i < vp.Current.End

		if mode == Unified {
			out = append(out, p.unifiedLine(doc, lines[i], numWidth, vp, current))
		} else {
			out = append(out, p.sideBySideLine(doc, lines[i], numWidth, vp, current))
		}
	}

	return out
}

// half is one side of a side-by-side line.
type half struct {
	v       version
	index   int  // line index, -1 for an empty side
	changed bool // this side is removed or added text
	sign    string
	signFg  color.Color
	bg      color.Color
	wordBg  color.Color
	words   []diff.Span
}

// sideBySideLine draws the old version on the left and the new on the right.
//
//	[sign][number] [code] │ [sign][number] [code]
func (p *Palette) sideBySideLine(doc *Doc, dl DisplayLine, numWidth int, vp Viewport, current bool) string {
	var left, right half
	var halfWidth int
	var codeWidth int
	var extra int
	var b strings.Builder

	halfWidth = (vp.Width - 1) / 2
	codeWidth = max(halfWidth-numWidth-2, 1)
	extra = vp.Width - 1 - 2*halfWidth

	left, right = p.halves(doc, dl)

	b.WriteString(p.drawHalf(left, numWidth, codeWidth, vp.XOffset, current))
	b.WriteString(p.Style(p.Border, p.Bg, false).Render("│"))
	b.WriteString(p.drawHalf(right, numWidth, codeWidth, vp.XOffset, current))

	// An odd width leaves one column at the end.
	if extra > 0 {
		b.WriteString(p.Style(nil, p.Bg, false).Render(strings.Repeat(" ", extra)))
	}

	return b.String()
}

// halves decides the colors and highlighted words of both sides of a row.
func (p *Palette) halves(doc *Doc, dl DisplayLine) (half, half) {
	var left, right half

	left = half{v: version{doc.Old, doc.OldSyntax}, index: dl.Old, bg: p.Bg, sign: " "}
	right = half{v: version{doc.New, doc.NewSyntax}, index: dl.New, bg: p.Bg, sign: " "}

	// Removed text on the left, added text on the right.
	if dl.Kind == diff.Removed || dl.Kind == diff.Modified {
		left.changed = true
		left.bg, left.wordBg = p.RemovedBg, p.RemovedWord
		left.sign, left.signFg = "-", p.RemovedFg
	}
	if dl.Kind == diff.Added || dl.Kind == diff.Modified {
		right.changed = true
		right.bg, right.wordBg = p.AddedBg, p.AddedWord
		right.sign, right.signFg = "+", p.AddedFg
	}

	// A modified pair highlights only the words that differ.
	if dl.Kind == diff.Modified {
		row := doc.Rows[dl.Row]
		left.words, right.words = diff.Words(doc.Old[row.Old], doc.New[row.New], doc.CharLevel)
	}

	return left, right
}

// drawHalf draws one side: sign, line number and code.
func (p *Palette) drawHalf(h half, numWidth, codeWidth, xoff int, current bool) string {
	var b strings.Builder
	var filler color.Color

	// No line on this side: fill it so the other side stands out.
	if h.index < 0 {
		filler = or(p.Filler, p.Bg)
		return p.Style(nil, filler, false).Render(strings.Repeat(" ", numWidth+2+codeWidth))
	}

	b.WriteString(p.Style(h.signFg, h.bg, true).Render(h.sign))
	b.WriteString(p.number(h.index, numWidth, h.bg, current))
	b.WriteString(p.Style(nil, h.bg, false).Render(" "))
	b.WriteString(p.code(cell{
		text:   h.v.lines[h.index],
		syntax: lineSyntax(h.v.syntax, h.index),
		words:  h.words,
		width:  codeWidth,
		xoff:   xoff,
		bg:     h.bg,
		wordBg: h.wordBg,
	}))

	return b.String()
}

// unifiedLine draws one line of the unified view:
//
//	[sign][old number] [new number] [code]
func (p *Palette) unifiedLine(doc *Doc, dl DisplayLine, numWidth int, vp Viewport, current bool) string {
	var left, right half
	var h half
	var codeWidth int
	var b strings.Builder

	codeWidth = max(vp.Width-2*numWidth-3, 1)
	left, right = p.halves(doc, dl)

	// Show the old side for removed lines and the old half of a modified
	// pair; the new side otherwise.
	h = right
	if dl.New < 0 {
		h = left
	}

	b.WriteString(p.Style(h.signFg, h.bg, true).Render(h.sign))
	b.WriteString(p.number(dl.Old, numWidth, h.bg, current))
	b.WriteString(p.Style(nil, h.bg, false).Render(" "))
	b.WriteString(p.number(dl.New, numWidth, h.bg, current))
	b.WriteString(p.Style(nil, h.bg, false).Render(" "))

	// In unified mode a modified pair is split in two lines; recompute
	// the words against the other half of the row.
	if dl.Kind == diff.Modified {
		row := doc.Rows[dl.Row]
		oldWords, newWords := diff.Words(doc.Old[row.Old], doc.New[row.New], doc.CharLevel)
		h.words = newWords
		if dl.New < 0 {
			h.words = oldWords
		}
	}

	b.WriteString(p.code(cell{
		text:   h.v.lines[h.index],
		syntax: lineSyntax(h.v.syntax, h.index),
		words:  h.words,
		width:  codeWidth,
		xoff:   vp.XOffset,
		bg:     h.bg,
		wordBg: h.wordBg,
	}))

	return b.String()
}

// number draws a right-aligned 1-based line number, or blanks for -1.
func (p *Palette) number(index, width int, bg color.Color, current bool) string {
	var text string

	text = strings.Repeat(" ", width)
	if index >= 0 {
		text = fmt.Sprintf("%*d", width, index+1)
	}

	if current {
		return p.Style(p.Cursor, bg, true).Render(text)
	}
	return p.Style(p.LineNumber, bg, false).Render(text)
}

// lineSyntax returns the spans of line i, or nil without highlighting.
func lineSyntax(spans [][]syntax.Span, i int) []syntax.Span {
	if i < len(spans) {
		return spans[i]
	}
	return nil
}
