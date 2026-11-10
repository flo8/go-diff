package diff

// Kind is what happened to one row of the side-by-side view.
type Kind uint8

const (
	Same     Kind = iota // unchanged, or changed only in ways the filters ignore
	Removed              // only in the old version
	Added                // only in the new version
	Modified             // a removed line paired with an added line
)

// Row is one line of the side-by-side view. Old and New are 0-based line
// indices, or -1 when that side has no line on this row.
type Row struct {
	Old  int
	New  int
	Kind Kind
}

// Side is one version of a file as seen by the aligner.
type Side struct {
	Keys    []string // comparison key of each line
	Ignored []bool   // line does not matter (comment-only, blank, ...)
}

// Counts are the meaningful changes in a file.
type Counts struct {
	Removed  int
	Added    int
	Modified int
}

// Total is the number of changed rows.
func (c Counts) Total() int {
	return c.Removed + c.Added + c.Modified
}

// Align lines up the old and new versions of a file.
//
// Ignored lines are left out of the comparison, so adding or removing a
// comment or a blank line is never a change. Lines whose keys are equal are
// matched with Match. What remains between two matches is a gap: inside a
// gap, the k-th changed old line is paired with the k-th changed new line
// as Modified, and the leftovers are Removed or Added.
func Align(old, new Side) []Row {
	var oldIdx, newIdx []int
	var pairs []Pair
	var rows []Row
	var prevOld, prevNew int

	// Compare only the lines that matter.
	oldIdx = significant(old)
	newIdx = significant(new)
	pairs = Match(pick(old.Keys, oldIdx), pick(new.Keys, newIdx))

	rows = make([]Row, 0, max(len(old.Keys), len(new.Keys)))

	// Emit each gap, then the matched line that closes it.
	for _, p := range pairs {
		o := oldIdx[p.A]
		n := newIdx[p.B]
		rows = appendGap(rows, old, new, prevOld, o, prevNew, n)
		rows = append(rows, Row{Old: o, New: n, Kind: Same})
		prevOld = o + 1
		prevNew = n + 1
	}

	// Lines after the last match.
	rows = appendGap(rows, old, new, prevOld, len(old.Keys), prevNew, len(new.Keys))

	return rows
}

// appendGap emits the rows for old lines [oi, oEnd) and new lines
// [ni, nEnd), none of which matched.
func appendGap(rows []Row, old, new Side, oi, oEnd, ni, nEnd int) []Row {
	for oi < oEnd || ni < nEnd {
		hasOld := oi < oEnd
		hasNew := ni < nEnd
		oldIgnored := hasOld && old.Ignored[oi]
		newIgnored := hasNew && new.Ignored[ni]

		switch {

		// Ignored lines are shown as context, side by side when both have one.
		case oldIgnored && newIgnored:
			rows = append(rows, Row{Old: oi, New: ni, Kind: Same})
			oi++
			ni++
		case oldIgnored:
			rows = append(rows, Row{Old: oi, New: -1, Kind: Same})
			oi++
		case newIgnored:
			rows = append(rows, Row{Old: -1, New: ni, Kind: Same})
			ni++

		// Real changes: pair them while both sides have one.
		case hasOld && hasNew:
			rows = append(rows, Row{Old: oi, New: ni, Kind: Modified})
			oi++
			ni++
		case hasOld:
			rows = append(rows, Row{Old: oi, New: -1, Kind: Removed})
			oi++
		default:
			rows = append(rows, Row{Old: -1, New: ni, Kind: Added})
			ni++
		}
	}

	return rows
}

// Count tallies the changed rows.
func Count(rows []Row) Counts {
	var c Counts

	for _, r := range rows {
		switch r.Kind {
		case Removed:
			c.Removed++
		case Added:
			c.Added++
		case Modified:
			c.Modified++
		}
	}

	return c
}

// significant returns the indices of the lines that are not ignored.
func significant(s Side) []int {
	var idx []int

	idx = make([]int, 0, len(s.Keys))
	for i := range s.Keys {
		if !s.Ignored[i] {
			idx = append(idx, i)
		}
	}

	return idx
}

// pick returns keys[i] for each i in idx.
func pick(keys []string, idx []int) []string {
	var out []string

	out = make([]string, len(idx))
	for i, k := range idx {
		out[i] = keys[k]
	}

	return out
}
