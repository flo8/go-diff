// Package diff compares sequences of lines and words.
//
// It knows nothing about languages or filters: callers hand it comparison
// keys (already normalized by package semantic) and get back an alignment.
package diff

import (
	"slices"
)

// Pair is an index into a and an index into b whose elements are equal.
type Pair struct {
	A int
	B int
}

// maxEditDistance bounds the work Myers' algorithm may do: its trace
// costs about 4*D*D bytes. Beyond it, the input is cut into smaller
// pieces at lines that are unique on both sides (see anchored).
const maxEditDistance = 3000

// Match returns a common subsequence of a and b as index pairs, in
// increasing order. It is the longest one unless the inputs are very
// different, in which case it is a close approximation.
func Match(a, b []string) []Pair {
	var ia, ib []int

	// Compare small integers instead of strings.
	ia, ib = intern(a, b)
	return matchInts(ia, ib)
}

// matchInts matches the equal prefix and suffix, then runs Myers'
// O(ND) algorithm on the middle part. If the middle part has too many
// differences, it is split at anchor lines instead.
func matchInts(ia, ib []int) []Pair {
	var prefix, suffix int
	var pairs []Pair
	var middle []Pair
	var ok bool

	// Common prefix.
	for prefix < len(ia) && prefix < len(ib) && ia[prefix] == ib[prefix] {
		pairs = append(pairs, Pair{prefix, prefix})
		prefix++
	}

	// Common suffix, not overlapping the prefix.
	for suffix < len(ia)-prefix && suffix < len(ib)-prefix &&
		ia[len(ia)-1-suffix] == ib[len(ib)-1-suffix] {
		suffix++
	}

	// The middle part, shifted back to absolute indices.
	midA := ia[prefix : len(ia)-suffix]
	midB := ib[prefix : len(ib)-suffix]
	middle, ok = myers(midA, midB)
	if !ok {
		middle = anchored(midA, midB)
	}
	for _, p := range middle {
		pairs = append(pairs, Pair{p.A + prefix, p.B + prefix})
	}

	for i := 0; i < suffix; i++ {
		pairs = append(pairs, Pair{len(ia) - suffix + i, len(ib) - suffix + i})
	}

	return pairs
}

// intern maps each distinct string to a small integer.
func intern(a, b []string) ([]int, []int) {
	var ids map[string]int
	var ia, ib []int

	ids = make(map[string]int, len(a)+len(b))
	ia = make([]int, len(a))
	ib = make([]int, len(b))

	for i, s := range a {
		ia[i] = internOne(ids, s)
	}
	for i, s := range b {
		ib[i] = internOne(ids, s)
	}

	return ia, ib
}

func internOne(ids map[string]int, s string) int {
	id, ok := ids[s]
	if !ok {
		id = len(ids)
		ids[s] = id
	}
	return id
}

// myers finds the longest common subsequence with the greedy forward
// algorithm from "An O(ND) Difference Algorithm and Its Variations"
// (Myers, 1986). v[k] holds the furthest x reached on diagonal k = x - y.
// Before each round d a copy of v[-d..d] is kept so the path can be
// walked back at the end. It returns false when the edit distance is
// over maxEditDistance.
func myers(a, b []int) ([]Pair, bool) {
	var n, m int
	var limit int
	var offset int
	var v []int32
	var trace [][]int32
	var x, y int

	n = len(a)
	m = len(b)
	if n == 0 || m == 0 {
		return nil, true
	}

	limit = min(n+m, maxEditDistance)
	offset = limit + 1
	v = make([]int32, 2*limit+3)

	for d := 0; d <= limit; d++ {

		// Remember the state before this round for the backtrack.
		snap := make([]int32, 2*d+1)
		copy(snap, v[offset-d:offset+d+1])
		trace = append(trace, snap)

		for k := -d; k <= d; k += 2 {

			// Step down (insertion) or right (deletion), whichever
			// reaches further.
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = int(v[offset+k+1])
			} else {
				x = int(v[offset+k-1]) + 1
			}
			y = x - k

			// Follow the diagonal of equal elements.
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = int32(x)

			if x >= n && y >= m {
				return backtrack(trace, n, m), true
			}
		}
	}

	// Too many differences.
	return nil, false
}

// backtrack walks the trace from (n, m) back to (0, 0) and collects the
// diagonal moves, which are the matched pairs.
func backtrack(trace [][]int32, n, m int) []Pair {
	var pairs []Pair
	var x, y int
	var prevK, prevX, prevY int

	x = n
	y = m

	for d := len(trace) - 1; d >= 0; d-- {
		snap := trace[d]
		k := x - y

		// Round 0 starts at (0, 0): everything left is a diagonal.
		if d == 0 {
			prevX = 0
			prevY = 0
		} else {
			if k == -d || (k != d && snap[k-1+d] < snap[k+1+d]) {
				prevK = k + 1
			} else {
				prevK = k - 1
			}
			prevX = int(snap[prevK+d])
			prevY = prevX - prevK
		}

		// The diagonal part of this round, walked backwards.
		for x > prevX && y > prevY {
			x--
			y--
			pairs = append(pairs, Pair{x, y})
		}

		x = prevX
		y = prevY
	}

	// Pairs were collected from the end.
	slices.Reverse(pairs)

	return pairs
}
