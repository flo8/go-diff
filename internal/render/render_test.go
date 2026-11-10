package render

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/theme"
)

// Scenario: a deleted function whose blank line and comment are ignored
// (shown on one side only), followed by context and a second change.
// Goal: the deletion is one block, the later change another.
func TestBlocksMergeAcrossOneSidedContext(t *testing.T) {
	rows := []diff.Row{
		{Old: 0, New: 0, Kind: diff.Same},
		{Old: 1, New: -1, Kind: diff.Removed},
		{Old: 2, New: -1, Kind: diff.Same}, // ignored blank line, old side only
		{Old: 3, New: -1, Kind: diff.Removed},
		{Old: 4, New: 1, Kind: diff.Same},
		{Old: 5, New: 2, Kind: diff.Modified},
	}

	got := Blocks(Layout(rows, SideBySide))
	want := []Block{{Start: 1, End: 4}, {Start: 5, End: 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}
}

// Scenario: unified layout of a modified row.
// Goal: it becomes two lines, old then new, and blocks cover both.
func TestUnifiedSplitsModifiedRows(t *testing.T) {
	rows := []diff.Row{
		{Old: 0, New: 0, Kind: diff.Same},
		{Old: 1, New: 1, Kind: diff.Modified},
	}

	lines := Layout(rows, Unified)
	if len(lines) != 3 || lines[1].New != -1 || lines[2].Old != -1 {
		t.Fatalf("lines = %+v", lines)
	}
	if got := Blocks(lines); !reflect.DeepEqual(got, []Block{{Start: 1, End: 3}}) {
		t.Errorf("blocks = %v", got)
	}
}

// Scenario: code cells with tabs, wide characters, long text and
// horizontal scrolling.
// Goal: every cell is exactly the requested width, so columns line up.
func TestCodeCellWidth(t *testing.T) {
	p := NewPalette(theme.Builtins()[0])
	texts := []string{
		"",
		"\tx := 1",
		"a\tb\tc",
		"日本語のテキスト",
		"a very long line that is much wider than the cell is allowed to be",
		"ctrl\x01char\r",
	}

	for _, text := range texts {
		for _, xoff := range []int{0, 1, 3} {
			out := p.code(cell{text: text, width: 10, xoff: xoff, bg: p.Bg})
			if w := ansi.StringWidth(out); w != 10 {
				t.Errorf("text %q xoff %d: width %d, want 10", text, xoff, w)
			}
		}
	}
}

// Scenario: drawing a small side-by-side and unified document.
// Goal: each screen line fills the viewport width exactly.
func TestDrawLineWidths(t *testing.T) {
	p := NewPalette(theme.Builtins()[1])
	doc := &Doc{
		Rows: []diff.Row{
			{Old: 0, New: 0, Kind: diff.Same},
			{Old: 1, New: 1, Kind: diff.Modified},
			{Old: -1, New: 2, Kind: diff.Added},
		},
		Old: []string{"package a", "var x = 500"},
		New: []string{"package a", "var x = 1000", "var y = 2"},
	}

	for _, mode := range []Mode{SideBySide, Unified} {
		for _, width := range []int{40, 41, 80} {
			lines := Layout(doc.Rows, mode)
			out := Draw(doc, lines, mode, Viewport{Height: 10, Width: width}, p)
			for i, l := range out {
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("mode %d width %d line %d: got width %d", mode, width, i, w)
				}
			}
		}
	}
}
