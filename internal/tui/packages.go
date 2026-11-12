package tui

import (
	"fmt"
	"path"

	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/review"
)

// maxPkgColWidth is the widest the Package column gets; longer
// directories are cut from the left.
const maxPkgColWidth = 32

// packageDir is the directory of a file, which for Go code is its
// package: "internal/tui", or "." at the top of the repository.
func packageDir(p string) string {
	return path.Dir(p)
}

// packageTotal counts the Go packages of the files shown in the table:
// the directories holding a changed .go file, tests included.
func (s *changeSet) packageTotal() int {
	var dirs map[string]bool
	var p string

	dirs = map[string]bool{}
	for _, f := range s.list.visible {
		p = f.Change.Path()
		if groupOf(p) != groupOther {
			dirs[packageDir(p)] = true
		}
	}

	return len(dirs)
}

// pkgColWidth fits the Package column to the longest directory of
// files, title included, plus two spaces before the file names.
func pkgColWidth(files []*review.File) int {
	var width int

	width = len("Package")
	for _, f := range files {
		width = max(width, ansi.StringWidth(packageDir(f.Change.Path())))
	}

	return min(width, maxPkgColWidth) + 2
}

// cutLeft cuts plain text to width columns from the left, so that the
// end stays visible: "…nal/tui".
func cutLeft(s string, width int) string {
	var runes []rune

	runes = []rune(s)
	if len(runes) > width {
		return "…" + string(runes[len(runes)-max(width-1, 0):])
	}
	return s
}

// packageCount formats "1 package" or "3 packages".
func packageCount(n int) string {
	if n == 1 {
		return "1 package"
	}
	return fmt.Sprintf("%d packages", n)
}
