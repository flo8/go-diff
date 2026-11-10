// Package semantic decides which parts of a file matter when comparing
// two versions of it.
//
// A language plugs in through the Tokenizer interface: it reports where
// the code, comment and import tokens are. Analyze then turns those
// tokens into one comparison key per line. Two lines with the same key
// are considered equal, and lines flagged Ignored (comment-only, blank)
// are left out of the comparison entirely.
//
// Nothing here modifies the source; it only reads it.
package semantic

import (
	"path/filepath"
	"sort"
	"strings"
)

// Filters choose which differences are ignored.
type Filters struct {
	Whitespace bool // indentation, spacing and blank lines
	Comments   bool // comment text and comment-only lines
	Formatting bool // spacing between tokens (needs a tokenizer)
	Imports    bool // import declarations (Go only for now)
}

// DefaultFilters is what a review starts with.
var DefaultFilters = Filters{Whitespace: true, Comments: true, Formatting: true}

// TokenKind classifies a token for filtering.
type TokenKind uint8

const (
	Code TokenKind = iota
	Comment
	Import
)

// Token is a non-whitespace piece of the source: src[Start:End].
type Token struct {
	Start int
	End   int
	Kind  TokenKind
}

// Tokenizer finds the tokens of one language. It must tolerate code that
// does not compile, since work in progress is reviewed too.
type Tokenizer interface {
	Tokens(src []byte) []Token
}

// Line is one source line with its comparison key.
type Line struct {
	Text    string // the line as written, without the line ending
	Key     string // what is compared
	Ignored bool   // the line does not take part in the comparison
}

// maxTokenizeSize is the largest file that is tokenized. Bigger files are
// compared as plain text so that huge generated files stay fast.
const maxTokenizeSize = 1 << 20

// TokenizerFor returns the tokenizer for a file name, or nil when the
// language is unknown and the file is handled as plain text.
func TokenizerFor(path string) Tokenizer {
	if strings.EqualFold(filepath.Ext(path), ".go") {
		return goTokenizer{}
	}
	return chromaTokenizerFor(path)
}

// Analyze splits src into lines and computes each line's key under the
// given filters.
func Analyze(path string, src []byte, f Filters) []Line {
	var tokenizer Tokenizer
	var tokens []Token
	var raw []string
	var cleaned []string
	var tokenKeys []string
	var lines []Line

	raw = splitLines(string(src))

	// Find the tokens, unless the language is unknown or the file is huge.
	tokenizer = TokenizerFor(path)
	if tokenizer != nil && len(src) <= maxTokenizeSize {
		tokens = tokenizer.Tokens(src)
	}

	// The source with ignored tokens removed, cut into lines.
	cleaned = splitLines(removeIgnored(src, tokens, f))

	// Token-level keys make spacing between tokens irrelevant.
	if f.Formatting && tokens != nil {
		tokenKeys = keysFromTokens(src, tokens, f, len(raw))
	}

	lines = make([]Line, len(raw))
	for i := range raw {
		lines[i] = makeLine(raw[i], cleaned[i], tokenKeys, i, f)
	}

	return lines
}

// makeLine picks the key for line i and decides whether it is ignored.
func makeLine(raw, cleaned string, tokenKeys []string, i int, f Filters) Line {
	var line Line
	var rawBlank bool
	var cleanedBlank bool

	line.Text = raw
	rawBlank = strings.TrimSpace(raw) == ""
	cleanedBlank = strings.TrimSpace(cleaned) == ""

	// A line that only held ignored tokens (a comment) never matters.
	// A line that was blank to begin with matters only when whitespace counts.
	line.Ignored = cleanedBlank && (!rawBlank || f.Whitespace || f.Formatting)

	switch {
	case tokenKeys != nil:
		line.Key = tokenKeys[i]
	case f.Whitespace || f.Formatting:
		line.Key = strings.Join(strings.Fields(cleaned), " ")
	default:
		line.Key = cleaned
	}

	// Adding a comma after the last item of a list, so that the next
	// item can be added, is formatting: `"tsc"` and `"tsc",` are equal.
	if f.Formatting {
		line.Key = trimTrailingComma(line.Key)
	}

	return line
}

// trimTrailingComma removes one comma at the end of a key, and the
// spaces around it.
func trimTrailingComma(key string) string {
	var trimmed string

	trimmed = strings.TrimRight(key, " \t")
	trimmed = strings.TrimSuffix(trimmed, ",")
	return strings.TrimRight(trimmed, " \t")
}

// isDropped reports whether a token is filtered out.
func isDropped(t Token, f Filters) bool {
	return (t.Kind == Comment && f.Comments) || (t.Kind == Import && f.Imports)
}

// removeIgnored returns src without the bytes of filtered tokens.
// Newlines inside removed tokens are kept so line numbers do not move.
func removeIgnored(src []byte, tokens []Token, f Filters) string {
	var b strings.Builder
	var pos int

	b.Grow(len(src))

	for _, t := range tokens {
		if !isDropped(t, f) {
			continue
		}

		// Copy what precedes the token, then only its newlines.
		b.Write(src[pos:t.Start])
		for _, c := range src[t.Start:t.End] {
			if c == '\n' {
				b.WriteByte('\n')
			}
		}
		pos = t.End
	}

	b.Write(src[pos:])
	return b.String()
}

// keysFromTokens builds each line's key by joining the kept tokens that
// start on (or continue onto) that line with single spaces.
func keysFromTokens(src []byte, tokens []Token, f Filters, numLines int) []string {
	var lineStarts []int
	var parts [][]string
	var keys []string

	lineStarts = findLineStarts(src)
	parts = make([][]string, numLines)

	for _, t := range tokens {
		if isDropped(t, f) {
			continue
		}

		// A token can span lines (raw strings): give each line its piece.
		line := lineOf(lineStarts, t.Start)
		pieces := strings.Split(string(src[t.Start:t.End]), "\n")
		for j, piece := range pieces {
			if line+j < numLines {
				parts[line+j] = append(parts[line+j], strings.TrimRight(piece, "\r"))
			}
		}
	}

	keys = make([]string, numLines)
	for i, p := range parts {
		keys[i] = strings.Join(p, " ")
	}

	return keys
}

// splitLines cuts text into lines without their "\n" or "\r\n" endings.
// A final newline does not start an extra empty line.
func splitLines(text string) []string {
	var lines []string

	if text == "" {
		return nil
	}

	text = strings.TrimSuffix(text, "\n")
	lines = strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}

	return lines
}

// findLineStarts returns the byte offset where each line begins.
func findLineStarts(src []byte) []int {
	var starts []int

	starts = []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}

	return starts
}

// lineOf returns the 0-based line that contains byte offset off.
func lineOf(lineStarts []int, off int) int {
	var next int

	next = sort.SearchInts(lineStarts, off+1)
	return next - 1
}
