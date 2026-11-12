package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
)

// changeSet is the files of one comparison (the working tree, or one
// commit) and the state of their table.
type changeSet struct {
	files     []*review.File
	rangeName string           // "HEAD → working tree", "a1b2c3d4^ → a1b2c3d4"
	filters   semantic.Filters // the filters the files were computed with
	list      fileList
}

// newChangeSet wraps files already computed with filters.
func newChangeSet(files []*review.File, rangeName string, filters semantic.Filters) *changeSet {
	var s *changeSet

	s = &changeSet{files: files, rangeName: rangeName, filters: filters}
	s.list.refresh(files)

	return s
}

// recompute brings the files up to date with the filters. Sets that are
// not on screen are recomputed only when they are shown again.
func (s *changeSet) recompute(f semantic.Filters) {
	if s.filters == f {
		return
	}

	review.ComputeAll(s.files, f)
	s.filters = f
	s.list.refresh(s.files)
}

// totals sums the counts of the files shown in the table.
func (s *changeSet) totals() diff.Counts {
	var total diff.Counts

	for _, f := range s.list.visible {
		total.Removed += f.Counts.Removed
		total.Added += f.Counts.Added
		total.Modified += f.Counts.Modified
	}

	return total
}

// funcTotal sums the changed functions of the files shown in the table.
func (s *changeSet) funcTotal() int {
	var total int

	for _, f := range s.list.visible {
		total += f.Funcs
	}

	return total
}

// baseName and targetName split the range description "A → B".
func (s *changeSet) baseName() string {
	base, _, _ := strings.Cut(s.rangeName, " → ")
	return base
}

func (s *changeSet) targetName() string {
	_, target, _ := strings.Cut(s.rangeName, " → ")
	return target
}

// pane is a file table and the diff viewer opened from it. Each tab has
// its own, so switching tabs keeps the place in both.
type pane struct {
	set    *changeSet // nil while a commit is still loading
	viewer *viewer    // nil when no file is open
}

// padTo cuts or pads a styled line to exactly width columns.
func padTo(line string, width int) string {
	var w int

	line = ansi.Truncate(line, width, "…")
	w = ansi.StringWidth(line)
	return line + strings.Repeat(" ", max(width-w, 0))
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(width-ansi.StringWidth(s), 0)) + s
}

// truncate cuts plain text to width columns.
func truncate(s string, width int) string {
	var runes []rune

	runes = []rune(s)
	if len(runes) > width {
		return string(runes[:max(width-1, 0)]) + "…"
	}
	return s
}
