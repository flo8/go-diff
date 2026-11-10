// Package syntax finds which parts of each line should be colored as
// keywords, strings, comments and so on, using Chroma lexers.
package syntax

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Class is a syntax color category. Themes give each class a color.
type Class uint8

const (
	Plain Class = iota
	Keyword
	Type
	Function
	String
	Number
	Comment
	Operator
)

// Span colors the bytes [Start, End) of one line.
type Span struct {
	Start int
	End   int
	Class Class
}

// maxHighlightSize is the largest file that gets syntax colors.
const maxHighlightSize = 1 << 20

// Highlight returns the colored spans of each line of src. Lines are
// split on "\n" like package semantic does. It returns nil when the
// language is unknown or the file is too large; callers then draw plain
// text.
func Highlight(path string, src []byte) [][]Span {
	var lexer chroma.Lexer
	var iter chroma.Iterator
	var lines [][]Span
	var line int
	var col int
	var err error

	if len(src) == 0 || len(src) > maxHighlightSize {
		return nil
	}

	lexer = lexers.Match(filepath.Base(path))
	if lexer == nil {
		return nil
	}

	iter, err = lexer.Tokenise(nil, string(src))
	if err != nil {
		return nil
	}

	lines = make([][]Span, strings.Count(string(src), "\n")+1)

	// Walk the tokens, cutting multi-line tokens at each newline.
	for t := iter(); t != chroma.EOF; t = iter() {
		class := classOf(t.Type)
		pieces := strings.Split(t.Value, "\n")

		for i, piece := range pieces {
			if i > 0 {
				line++
				col = 0
			}
			if line >= len(lines) {
				break
			}
			if class != Plain && piece != "" {
				lines[line] = append(lines[line], Span{Start: col, End: col + len(piece), Class: class})
			}
			col += len(piece)
		}
	}

	return lines
}

// classOf maps Chroma's detailed token types to our few classes.
func classOf(t chroma.TokenType) Class {
	switch {
	case t.InSubCategory(chroma.CommentPreproc):
		return Keyword
	case t.InCategory(chroma.Comment):
		return Comment
	case t == chroma.KeywordType || t == chroma.NameClass || t == chroma.NameNamespace:
		return Type
	case t == chroma.KeywordConstant:
		return Number
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic || t == chroma.NameBuiltin:
		return Function
	case t.InSubCategory(chroma.LiteralString):
		return String
	case t.InSubCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Operator):
		return Operator
	}
	return Plain
}
