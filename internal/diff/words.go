package diff

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Span is a byte range [Start, End) inside a line.
type Span struct {
	Start int
	End   int
}

// token is a byte range of a line that is compared as one unit.
type token struct {
	start int
	end   int
}

// Words finds the parts of two versions of a line that differ.
//
// In word mode the lines are split into identifiers/numbers, whitespace
// runs and single punctuation characters, which works for most programming
// languages. In character mode (charLevel) every rune is its own token,
// which is the fallback for unknown file types.
//
// Whitespace alone is never highlighted, and changed tokens separated
// only by whitespace are merged into one span.
func Words(a, b string, charLevel bool) ([]Span, []Span) {
	var ta, tb []token
	var pairs []Pair
	var changedA, changedB []bool

	ta = tokenize(a, charLevel)
	tb = tokenize(b, charLevel)
	pairs = Match(tokenTexts(a, ta), tokenTexts(b, tb))

	// Every token not part of the common subsequence changed.
	changedA = make([]bool, len(ta))
	changedB = make([]bool, len(tb))
	for i := range changedA {
		changedA[i] = true
	}
	for i := range changedB {
		changedB[i] = true
	}
	for _, p := range pairs {
		changedA[p.A] = false
		changedB[p.B] = false
	}

	return spans(a, ta, changedA), spans(b, tb, changedB)
}

// tokenize splits a line into tokens.
func tokenize(s string, charLevel bool) []token {
	var tokens []token
	var i int

	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		end := i + size

		// In word mode, extend runs of word characters or whitespace.
		if !charLevel && (isWord(r) || unicode.IsSpace(r)) {
			end = runEnd(s, end, r)
		}

		tokens = append(tokens, token{start: i, end: end})
		i = end
	}

	return tokens
}

// runEnd extends a run of the same class as first, starting at i.
func runEnd(s string, i int, first rune) int {
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if isWord(first) != isWord(r) || unicode.IsSpace(first) != unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return i
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func tokenTexts(s string, tokens []token) []string {
	var texts []string

	texts = make([]string, len(tokens))
	for i, t := range tokens {
		texts[i] = s[t.start:t.end]
	}

	return texts
}

// spans turns changed tokens into highlighted ranges.
func spans(s string, tokens []token, changed []bool) []Span {
	var out []Span

	for i, t := range tokens {
		text := s[t.start:t.end]

		// Pure whitespace is not highlighted on its own.
		if !changed[i] || strings.TrimSpace(text) == "" {
			continue
		}

		// Merge with the previous span when only whitespace separates them.
		last := len(out) - 1
		if last >= 0 && strings.TrimSpace(s[out[last].End:t.start]) == "" {
			out[last].End = t.end
			continue
		}

		out = append(out, Span{Start: t.start, End: t.end})
	}

	return out
}
