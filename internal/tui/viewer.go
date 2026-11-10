package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/flo8/go-diff/internal/render"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/syntax"
)

// viewerChrome is the number of lines around the diff: the header, the
// side titles and the footer.
const viewerChrome = 3

// panStep is how many columns left/right scroll.
const panStep = 8

// viewer is the state of the diff screen for one file.
type viewer struct {
	file    *review.File
	doc     *render.Doc
	mode    render.Mode
	lines   []render.DisplayLine
	blocks  []render.Block
	top     int // first display line on screen
	xoff    int // columns scrolled to the right
	maxXoff int
	current int // index into blocks, -1 when there are none
}

// newViewer opens a file and shows its first change.
func newViewer(f *review.File, screenHeight int) *viewer {
	var v *viewer

	v = &viewer{file: f, mode: render.SideBySide, current: -1}
	v.build()
	if len(v.blocks) > 0 {
		v.jump(0, screenHeight)
	}

	return v
}

// build creates the document and its layout from the file's last Compute.
func (v *viewer) build() {
	var longest int

	f := v.file
	v.doc = &render.Doc{
		Rows:      f.Rows,
		Old:       lineTexts(f.OldLines),
		New:       lineTexts(f.NewLines),
		OldSyntax: syntax.Highlight(f.Change.OldPath, f.Old),
		NewSyntax: syntax.Highlight(f.Change.NewPath, f.New),
		CharLevel: semantic.TokenizerFor(f.Change.Path()) == nil,
	}

	// Allow panning up to the longest line.
	for _, l := range v.doc.Old {
		longest = max(longest, utf8.RuneCountInString(l))
	}
	for _, l := range v.doc.New {
		longest = max(longest, utf8.RuneCountInString(l))
	}
	v.maxXoff = longest

	v.layout()
}

// layout computes the display lines for the current mode.
func (v *viewer) layout() {
	v.lines = render.Layout(v.doc.Rows, v.mode)
	v.blocks = render.Blocks(v.lines)
	v.current = min(v.current, len(v.blocks)-1)
}

// reload rebuilds the view after the filters changed, staying near the
// same place in the file.
func (v *viewer) reload(screenHeight int) {
	var row int

	row = v.topRow()
	v.build()
	v.top = render.LineForRow(v.lines, row)
	v.current = -1
	v.clamp(screenHeight)
}

// topRow is the row shown at the top of the screen.
func (v *viewer) topRow() int {
	if v.top < len(v.lines) {
		return v.lines[v.top].Row
	}
	return 0
}

// toggleMode switches between side-by-side and unified layouts.
func (v *viewer) toggleMode(screenHeight int) {
	var row int

	row = v.topRow()
	v.mode = render.Unified - v.mode
	v.layout()
	v.top = render.LineForRow(v.lines, row)
	v.clamp(screenHeight)
}

// bodyHeight is the number of diff lines that fit on screen.
func bodyHeight(screenHeight int) int {
	return max(screenHeight-viewerChrome, 1)
}

// clamp keeps the scroll position inside the document.
func (v *viewer) clamp(screenHeight int) {
	v.top = min(v.top, len(v.lines)-bodyHeight(screenHeight))
	v.top = max(v.top, 0)
	v.xoff = min(max(v.xoff, 0), v.maxXoff)
}

// jump makes block i current and scrolls it a quarter down the screen.
func (v *viewer) jump(i, screenHeight int) {
	v.current = i
	v.top = v.blocks[i].Start - bodyHeight(screenHeight)/4
	v.clamp(screenHeight)
}

// currentVisible reports whether the current change is on screen.
func (v *viewer) currentVisible(screenHeight int) bool {
	if v.current < 0 {
		return false
	}
	start := v.blocks[v.current].Start
	return start >= v.top && start < v.top+bodyHeight(screenHeight)
}

// next jumps to the next change. After scrolling away from the current
// change, "next" means the first change from the top of the screen.
func (v *viewer) next(screenHeight int) {
	if v.currentVisible(screenHeight) {
		if v.current+1 < len(v.blocks) {
			v.jump(v.current+1, screenHeight)
		}
		return
	}

	for i, b := range v.blocks {
		if b.Start >= v.top {
			v.jump(i, screenHeight)
			return
		}
	}
}

// prev jumps to the previous change, the same way next does.
func (v *viewer) prev(screenHeight int) {
	if v.currentVisible(screenHeight) {
		if v.current > 0 {
			v.jump(v.current-1, screenHeight)
		}
		return
	}

	for i := len(v.blocks) - 1; i >= 0; i-- {
		if v.blocks[i].Start < v.top {
			v.jump(i, screenHeight)
			return
		}
	}
}

// viewerKey handles keys on the diff screen of a pane.
func (m *Model) viewerKey(p *pane, key string) {
	var v *viewer
	var page int

	v = p.viewer
	page = bodyHeight(m.height)

	switch key {
	case "esc", "backspace":
		p.viewer = nil
		return
	case "down", "j":
		v.top++
	case "up", "k":
		v.top--
	case "pgdown", "space", "ctrl+f":
		v.top += page
	case "pgup", "ctrl+b":
		v.top -= page
	case "ctrl+d":
		v.top += page / 2
	case "ctrl+u":
		v.top -= page / 2
	case "home", "g":
		v.top = 0
	case "end", "G":
		v.top = len(v.lines)
	case "right", "l":
		v.xoff += panStep
	case "left", "h":
		v.xoff -= panStep
	case "n":
		v.next(m.height)
	case "N", "p":
		v.prev(m.height)
	case "s":
		v.toggleMode(m.height)
	case "]":
		m.otherFile(p, 1)
		return
	case "[":
		m.otherFile(p, -1)
		return
	}

	v.clamp(m.height)
}

// otherFile opens the next or previous file of the pane's table,
// keeping the side-by-side or unified mode.
func (m *Model) otherFile(p *pane, delta int) {
	var mode render.Mode

	mode = p.viewer.mode
	p.set.list.move(delta)
	m.openSelected(p)

	if p.viewer != nil && p.viewer.mode != mode {
		p.viewer.toggleMode(m.height)
		if len(p.viewer.blocks) > 0 {
			p.viewer.jump(0, m.height)
		}
	}
}

// viewerView draws the header, the side titles, the diff and the footer.
func (m *Model) viewerView(p *pane) string {
	var v *viewer
	var height int
	var lines []string
	var vp render.Viewport

	v = p.viewer
	height = bodyHeight(m.height)

	lines = append(lines, m.headerLine(viewerTitle(v)))
	lines = append(lines, m.sideTitles(p))

	vp = render.Viewport{Top: v.top, Height: height, Width: m.width, XOffset: v.xoff}
	if v.current >= 0 {
		vp.Current = v.blocks[v.current]
	}

	// Binary files have nothing to draw.
	if v.file.Binary {
		lines = append(lines, m.pal.Paint("  Binary file: contents are not compared.", m.pal.Muted))
	} else {
		lines = append(lines, render.Draw(v.doc, v.lines, v.mode, vp, m.pal)...)
	}

	for len(lines) < height+2 {
		lines = append(lines, "")
	}

	lines = append(lines, m.viewerFooter(v))
	return strings.Join(lines, "\n")
}

// viewerTitle shows the path, its counts and the change position.
func viewerTitle(v *viewer) string {
	var pos string

	pos = "no changes"
	if len(v.blocks) > 0 {
		pos = fmt.Sprintf("change %d/%d", max(v.current+1, 0), len(v.blocks))
	}

	return fmt.Sprintf("%s   -%d +%d ~%d   %s",
		v.file.Change.Path(), v.file.Counts.Removed, v.file.Counts.Added, v.file.Counts.Modified, pos)
}

// sideTitles names the two versions above the diff.
func (m *Model) sideTitles(p *pane) string {
	var c = p.viewer.file.Change
	var oldName, newName string
	var half int

	oldName = sideName(p.set.baseName(), c.OldPath)
	newName = sideName(p.set.targetName(), c.NewPath)

	// Unified view: one title for both.
	if p.viewer.mode == render.Unified {
		return m.pal.Style(m.pal.Muted, m.pal.Bg, true).Render(padTo(" "+oldName+"  →  "+newName, m.width))
	}

	half = (m.width - 1) / 2
	return m.pal.Style(m.pal.Muted, m.pal.Bg, true).Render(
		padRight(truncate(" "+oldName, half), half) + "│" + truncate(" "+newName, half))
}

// sideName is "HEAD: path", or a note that the file does not exist there.
func sideName(side, path string) string {
	if path == "" {
		return side + ": (no file)"
	}
	return side + ": " + path
}

// viewerFooter lists the main keys of the diff screen.
func (m *Model) viewerFooter(v *viewer) string {
	var modeName string

	modeName = "unified"
	if v.mode == render.Unified {
		modeName = "side-by-side"
	}

	return m.keyHints([][2]string{
		{"n/N", "next/prev change"}, {"j/k", "scroll"}, {"←→", "pan"},
		{"s", modeName}, {"[ ]", "prev/next file"}, {"esc", "back"}, {"tab", "switch tab"}, {"?", "help"},
	})
}

// lineTexts extracts the line texts.
func lineTexts(lines []semantic.Line) []string {
	var out []string

	out = make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}

	return out
}
