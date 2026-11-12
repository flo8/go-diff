package semantic

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// Func is one top-level function or method and the lines it covers,
// 0-based and inclusive. The doc comment is not part of it.
type Func struct {
	Name      string // "Run", or "T.Run" for a method on T or *T
	FirstLine int
	LastLine  int
}

// GoFuncs returns the functions of a Go file, in source order. It
// returns nil for other files and for files over the tokenize limit.
// Code that does not compile still yields the functions the parser
// could read.
func GoFuncs(path string, src []byte) []Func {
	var fset *token.FileSet
	var file *ast.File
	var funcs []Func

	if !strings.EqualFold(filepath.Ext(path), ".go") || len(src) > maxTokenizeSize {
		return nil
	}

	fset = token.NewFileSet()
	file, _ = parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if file == nil {
		return nil
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		funcs = append(funcs, Func{
			Name:      funcName(fn),
			FirstLine: fset.Position(fn.Pos()).Line - 1,
			LastLine:  fset.Position(fn.End()).Line - 1,
		})
	}

	return funcs
}

// funcName names a function, with its receiver type for a method so
// that T.Close and U.Close are told apart.
func funcName(fn *ast.FuncDecl) string {
	var recv ast.Expr

	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}

	// Strip the pointer and type parameters: (*T[K]) → T.
	recv = fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	switch r := recv.(type) {
	case *ast.IndexExpr:
		recv = r.X
	case *ast.IndexListExpr:
		recv = r.X
	}

	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
