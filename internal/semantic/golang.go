package semantic

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
)

// goTokenizer reads Go source with go/scanner, which accepts code that
// does not compile, and finds import declarations with go/parser.
type goTokenizer struct{}

// importRange is the byte range of one import declaration.
type importRange struct {
	start int
	end   int
}

// Tokens returns every Go token, marking comments and tokens that are
// part of an import declaration.
func (goTokenizer) Tokens(src []byte) []Token {
	var fset *token.FileSet
	var file *token.File
	var s scanner.Scanner
	var imports []importRange
	var tokens []Token

	fset = token.NewFileSet()
	file = fset.AddFile("", fset.Base(), len(src))
	s.Init(file, src, nil, scanner.ScanComments)
	imports = goImports(src)

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		// Semicolons inserted by the scanner at line ends are not in the source.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		start := file.Offset(pos)
		end := goTokenEnd(src, start, tok, lit)
		tokens = append(tokens, Token{Start: start, End: end, Kind: goKind(tok, start, imports)})
	}

	return tokens
}

// goKind classifies a scanned token.
func goKind(tok token.Token, start int, imports []importRange) TokenKind {
	if tok == token.COMMENT {
		return Comment
	}

	for _, r := range imports {
		if start >= r.start && start < r.end {
			return Import
		}
	}

	return Code
}

// goTokenEnd finds where a token ends in src. The scanner drops '\r' from
// comments and raw strings, so their literal can be shorter than the
// source; those are measured on the source instead.
func goTokenEnd(src []byte, start int, tok token.Token, lit string) int {
	var idx int

	switch {

	// "/* ... */": search for the closing marker.
	case tok == token.COMMENT && bytes.HasPrefix(src[start:], []byte("/*")):
		idx = bytes.Index(src[start+2:], []byte("*/"))
		if idx >= 0 {
			return start + 2 + idx + 2
		}
		return len(src)

	// "// ...": runs to the end of the line.
	case tok == token.COMMENT:
		idx = bytes.IndexByte(src[start:], '\n')
		if idx >= 0 {
			return start + idx
		}
		return len(src)

	// `raw string`: search for the closing backquote.
	case tok == token.STRING && len(lit) > 0 && lit[0] == '`':
		idx = bytes.IndexByte(src[start+1:], '`')
		if idx >= 0 {
			return start + 1 + idx + 1
		}
		return len(src)
	}

	// Operators have no literal; their text is the token's name.
	if lit == "" {
		return start + len(tok.String())
	}
	return start + len(lit)
}

// goImports returns the byte ranges of the import declarations. Only the
// file header is parsed (parser.ImportsOnly), so errors further down in
// the file do not matter.
func goImports(src []byte) []importRange {
	var fset *token.FileSet
	var file *ast.File
	var ranges []importRange

	fset = token.NewFileSet()
	file, _ = parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if file == nil {
		return nil
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		ranges = append(ranges, importRange{
			start: fset.Position(gen.Pos()).Offset,
			end:   fset.Position(gen.End()).Offset,
		})
	}

	return ranges
}
