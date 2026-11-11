package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// fileGroup is the kind of a file when the tables are grouped (o):
// Go code first, then Go tests, then everything else.
type fileGroup int

const (
	groupGo fileGroup = iota
	groupTests
	groupOther
	groupCount // number of groups
)

// groupNames label the separator line above each group.
var groupNames = [groupCount]string{"go", "tests", "other"}

// groupOf puts a path in its group.
func groupOf(path string) fileGroup {
	if strings.HasSuffix(path, "_test.go") {
		return groupTests
	}
	if strings.HasSuffix(path, ".go") {
		return groupGo
	}
	return groupOther
}

// startsGroup reports whether row i of a grouped list is the first file
// of its group, so a separator goes above it.
func (l *fileList) startsGroup(i int) bool {
	var path string
	var prev string

	if i == 0 {
		return true
	}
	path = l.visible[i].Change.Path()
	prev = l.visible[i-1].Change.Path()

	return groupOf(path) != groupOf(prev)
}

// groupSeparator draws the line above a group: its name and number of
// files, then a rule to the end of the width.
func (m *Model) groupSeparator(g fileGroup, files, width int) string {
	var label string
	var fill int

	label = fmt.Sprintf(" %s (%d) ", groupNames[g], files)
	fill = max(width-3-ansi.StringWidth(label), 0)

	return m.pal.Paint(" ──", m.pal.Border) +
		m.pal.Paint(label, m.pal.Muted) +
		m.pal.Paint(strings.Repeat("─", fill), m.pal.Border)
}
