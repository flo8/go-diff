package diff

// anchored matches two very different sequences without Myers' quadratic
// memory. Elements that appear exactly once in a and exactly once in b
// are almost always the "same line" on both sides; the longest chain of
// them that keeps its order on both sides becomes a set of anchors (this
// is the idea behind patience diff). The pieces between anchors are much
// smaller and are matched recursively with matchInts.
//
// When there are no unique common elements, nothing is matched.
func anchored(a, b []int) []Pair {
	var anchors []Pair
	var pairs []Pair
	var prevA, prevB int

	anchors = longestChain(uniqueCommon(a, b))
	if len(anchors) == 0 {
		return nil
	}

	// Match each piece between two anchors, then the anchor itself.
	for _, anchor := range anchors {
		pairs = appendShifted(pairs, matchInts(a[prevA:anchor.A], b[prevB:anchor.B]), prevA, prevB)
		pairs = append(pairs, anchor)
		prevA = anchor.A + 1
		prevB = anchor.B + 1
	}

	// The piece after the last anchor.
	pairs = appendShifted(pairs, matchInts(a[prevA:], b[prevB:]), prevA, prevB)

	return pairs
}

// appendShifted appends pairs moved by (da, db).
func appendShifted(pairs, more []Pair, da, db int) []Pair {
	for _, p := range more {
		pairs = append(pairs, Pair{A: p.A + da, B: p.B + db})
	}
	return pairs
}

// uniqueCommon returns the positions of elements that occur exactly
// once in a and exactly once in b, ordered by their position in a.
func uniqueCommon(a, b []int) []Pair {
	var countA, countB map[int]int
	var posB map[int]int
	var pairs []Pair

	countA = make(map[int]int, len(a))
	countB = make(map[int]int, len(b))
	posB = make(map[int]int, len(b))

	for _, x := range a {
		countA[x]++
	}
	for i, x := range b {
		countB[x]++
		posB[x] = i
	}

	for i, x := range a {
		if countA[x] == 1 && countB[x] == 1 {
			pairs = append(pairs, Pair{A: i, B: posB[x]})
		}
	}

	return pairs
}

// longestChain returns the longest subsequence of pairs (already sorted
// by A) whose B values also increase. This is the classic longest
// increasing subsequence with patience sorting, in O(n log n):
// tails[k] is the index of the smallest B ending a chain of length k+1.
func longestChain(pairs []Pair) []Pair {
	var tails []int
	var prev []int
	var chain []Pair
	var i int

	prev = make([]int, len(pairs))

	for i, p := range pairs {

		// Extend the longest chain whose tail is smaller than p.B.
		k := searchTail(pairs, tails, p.B)
		prev[i] = -1
		if k > 0 {
			prev[i] = tails[k-1]
		}

		if k == len(tails) {
			tails = append(tails, i)
		} else {
			tails[k] = i
		}
	}

	if len(tails) == 0 {
		return nil
	}

	// Walk back from the end of the longest chain.
	chain = make([]Pair, len(tails))
	i = tails[len(tails)-1]
	for k := len(tails) - 1; k >= 0; k-- {
		chain[k] = pairs[i]
		i = prev[i]
	}

	return chain
}

// searchTail returns the first k where pairs[tails[k]].B >= b.
// The B values at tails are increasing, so a binary search works.
func searchTail(pairs []Pair, tails []int, b int) int {
	var lo, hi int

	lo = 0
	hi = len(tails)
	for lo < hi {
		mid := (lo + hi) / 2
		if pairs[tails[mid]].B < b {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	return lo
}
