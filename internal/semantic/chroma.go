package semantic

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// chromaTokenizer uses a Chroma lexer to find comments in the many
// languages Chroma knows. Chroma lexers are regular-expression based and
// never fail on broken code.
type chromaTokenizer struct {
	lexer chroma.Lexer
}

// chromaTokenizerFor returns a tokenizer for the file name, or nil when
// Chroma has no lexer for it (or only the plain text one).
func chromaTokenizerFor(path string) Tokenizer {
	var lexer chroma.Lexer

	lexer = lexers.Match(filepath.Base(path))
	if lexer == nil || lexer.Config().Name == "plaintext" {
		return nil
	}

	return chromaTokenizer{lexer: lexer}
}

// Tokens returns the non-whitespace tokens with their kind. If the lexer
// changed the text in any way (some add a final newline), the offsets
// cannot be trusted and nil is returned: the file is then compared as
// plain text.
func (c chromaTokenizer) Tokens(src []byte) []Token {
	var iter chroma.Iterator
	var tokens []Token
	var off int
	var err error

	iter, err = c.lexer.Tokenise(nil, string(src))
	if err != nil {
		return nil
	}

	for t := iter(); t != chroma.EOF; t = iter() {
		end := off + len(t.Value)

		// The lexer may add a final newline the source does not have.
		if end == len(src)+1 && strings.HasSuffix(t.Value, "\n") {
			end = len(src)
			t.Value = t.Value[:len(t.Value)-1]
		}

		// The token must be exactly the source bytes at this offset.
		if end > len(src) || string(src[off:end]) != t.Value {
			return nil
		}

		if strings.TrimSpace(t.Value) != "" {
			tokens = append(tokens, Token{Start: off, End: end, Kind: chromaKind(t.Type)})
		}
		off = end
	}

	return tokens
}

// chromaKind maps a Chroma token type to a filter kind. Preprocessor
// lines such as "#include" are tagged as comments by Chroma but are code.
func chromaKind(t chroma.TokenType) TokenKind {
	if t.InCategory(chroma.Comment) && !t.InSubCategory(chroma.CommentPreproc) {
		return Comment
	}
	return Code
}
