package render

import (
	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/syntax"
)

// Mode is the diff layout.
type Mode uint8

const (
	SideBySide Mode = iota
	Unified
)

// Doc is everything needed to draw one file.
type Doc struct {
	Rows      []diff.Row
	Old       []string // old line texts
	New       []string // new line texts
	OldSyntax [][]syntax.Span
	NewSyntax [][]syntax.Span
	CharLevel bool // compare words character by character (unknown language)
}

// DisplayLine is one screen line. In side-by-side mode it is a whole Row.
// In unified mode a Modified row becomes two lines: the old one (New = -1)
// followed by the new one (Old = -1).
type DisplayLine struct {
	Row  int // index into Doc.Rows
	Old  int // old line index, or -1
	New  int // new line index, or -1
	Kind diff.Kind
}

// Block is a run of changed display lines [Start, End).
type Block struct {
	Start int
	End   int
}

// Layout lists the screen lines of a document in a mode.
func Layout(rows []diff.Row, mode Mode) []DisplayLine {
	var lines []DisplayLine

	lines = make([]DisplayLine, 0, len(rows))

	for i, r := range rows {
		// Unified mode shows a modified pair as two lines.
		if mode == Unified && r.Kind == diff.Modified {
			lines = append(lines, DisplayLine{Row: i, Old: r.Old, New: -1, Kind: r.Kind})
			lines = append(lines, DisplayLine{Row: i, Old: -1, New: r.New, Kind: r.Kind})
			continue
		}

		lines = append(lines, DisplayLine{Row: i, Old: r.Old, New: r.New, Kind: r.Kind})
	}

	return lines
}

// Blocks finds the runs of changed lines, which "n" and "N" jump between.
//
// Ignored lines shown on one side only (a blank or comment line inside a
// deleted function) do not end a run, so a deleted block is one change
// rather than one change per paragraph.
func Blocks(lines []DisplayLine) []Block {
	var blocks []Block
	var start int
	var lastChanged int

	start = -1
	for i, l := range lines {
		switch {
		case l.Kind != diff.Same:
			if start < 0 {
				start = i
			}
			lastChanged = i

		// One-sided context: keep the run open.
		case l.Old < 0 || l.New < 0:

		// Context on both sides closes the run.
		case start >= 0:
			blocks = append(blocks, Block{Start: start, End: lastChanged + 1})
			start = -1
		}
	}

	if start >= 0 {
		blocks = append(blocks, Block{Start: start, End: lastChanged + 1})
	}

	return blocks
}

// LineForRow returns the first display line showing a row, used to keep
// the position when switching modes.
func LineForRow(lines []DisplayLine, row int) int {
	for i, l := range lines {
		if l.Row >= row {
			return i
		}
	}
	return max(len(lines)-1, 0)
}
