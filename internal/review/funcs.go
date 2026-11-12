package review

import (
	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/semantic"
)

// countChangedFuncs returns how many functions have at least one
// changed row. Rows only change on code, so comment, blank and
// formatting edits never count.
//
// A removed line belongs to a function of the old version, an added
// line to one of the new version. A modified line is counted on the
// new side only, so renaming a function counts it once.
func countChangedFuncs(rows []diff.Row, oldFuncs, newFuncs []semantic.Func) int {
	var changed map[string]bool
	var name string

	changed = map[string]bool{}

	for _, r := range rows {
		switch r.Kind {
		case diff.Removed:
			name = funcAt(oldFuncs, r.Old)
		case diff.Added, diff.Modified:
			name = funcAt(newFuncs, r.New)
		default:
			continue
		}

		// Lines outside any function (types, vars, imports) do not count.
		if name != "" {
			changed[name] = true
		}
	}

	return len(changed)
}

// funcAt returns the name of the function that holds line, or "".
func funcAt(funcs []semantic.Func, line int) string {
	for _, f := range funcs {
		if line >= f.FirstLine && line <= f.LastLine {
			return f.Name
		}
	}
	return ""
}
