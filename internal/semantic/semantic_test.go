package semantic

import (
	"testing"

	"github.com/flo8/go-diff/internal/diff"
)

// countChanges analyzes two versions of a file and counts the changes the
// way the summary table does.
func countChanges(path, old, new string, f Filters) diff.Counts {
	oldLines := Analyze(path, []byte(old), f)
	newLines := Analyze(path, []byte(new), f)
	return diff.Count(diff.Align(toSide(oldLines), toSide(newLines)))
}

func toSide(lines []Line) diff.Side {
	var s diff.Side
	for _, l := range lines {
		s.Keys = append(s.Keys, l.Key)
		s.Ignored = append(s.Ignored, l.Ignored)
	}
	return s
}

const goBefore = `package main

import "fmt"

// Greet says hello.
func Greet(name string) {
	fmt.Println("hello", name)
}
`

// Scenario: only Go comments changed: a line comment was edited, a
// multi-line block comment and a trailing comment were added.
// Goal: with the comment filter on, nothing is counted.
func TestGoCommentOnlyChangesAreIgnored(t *testing.T) {
	after := `package main

import "fmt"

/*
Greet says hello
to someone.
*/
func Greet(name string) { // entry point
	fmt.Println("hello", name) // print it
}
`
	got := countChanges("main.go", goBefore, after, DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}
}

// Scenario: same comment-only edit with the comment filter off.
// Goal: the comment changes are counted.
func TestGoCommentsCountWhenFilterOff(t *testing.T) {
	after := `package main

import "fmt"

// Greet says hi.
func Greet(name string) {
	fmt.Println("hello", name)
}
`
	f := DefaultFilters
	f.Comments = false

	got := countChanges("main.go", goBefore, after, f)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts = %+v, want 1 modified", got)
	}
}

// Scenario: reformatting only: spaces around operators, re-indentation
// and extra blank lines.
// Goal: with the formatting filter on, nothing is counted.
func TestGoFormattingOnlyChangesAreIgnored(t *testing.T) {
	before := "package main\n\nfunc f(a,b int) int {\n\treturn a+b\n}\n"
	after := "package main\n\n\nfunc f(a, b int) int {\n        return a + b\n\n}\n"

	got := countChanges("main.go", before, after, DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}
}

// Scenario: a real code change hidden among comment and spacing changes.
// Goal: exactly one modified line is counted.
func TestGoRealChangeIsCounted(t *testing.T) {
	after := `package main

import "fmt"

// Greet says hello politely.
func Greet(name string) {
    fmt.Println("good morning", name)
}
`
	got := countChanges("main.go", goBefore, after, DefaultFilters)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts = %+v, want 1 modified", got)
	}
}

// Scenario: a "//" inside a string literal is not a comment.
// Goal: changing the text after it is a real change.
func TestGoCommentMarkerInsideString(t *testing.T) {
	before := "package main\n\nvar u = \"http://a.example\"\n"
	after := "package main\n\nvar u = \"http://b.example\"\n"

	got := countChanges("main.go", before, after, DefaultFilters)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts = %+v, want 1 modified", got)
	}
}

// Scenario: only the import block changed.
// Goal: counted by default, ignored with the import filter on.
func TestGoImportFilter(t *testing.T) {
	after := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\n// Greet says hello.\nfunc Greet(name string) {\n\tfmt.Println(\"hello\", name)\n}\n"

	got := countChanges("main.go", goBefore, after, DefaultFilters)
	if got.Total() == 0 {
		t.Errorf("import change not counted with default filters")
	}

	f := DefaultFilters
	f.Imports = true
	got = countChanges("main.go", goBefore, after, f)
	if got.Total() != 0 {
		t.Errorf("counts = %+v with import filter, want none", got)
	}
}

// Scenario: Go file that does not compile (work in progress).
// Goal: comments are still recognized and ignored.
func TestGoBrokenCodeStillFiltersComments(t *testing.T) {
	before := "package main\n\nfunc f( {\n\tx := \n}\n"
	after := "package main\n\n// broken for now\nfunc f( {\n\tx := \n}\n"

	got := countChanges("main.go", before, after, DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}
}

// Scenario: Python file where a comment changed and an indentation change
// happened on a code line.
// Goal: with default filters nothing is counted; without the whitespace and
// formatting filters, the indentation change is counted.
func TestPythonCommentsAndWhitespace(t *testing.T) {
	before := "def f():\n    # old note\n    return 1\n"
	after := "def f():\n    # new note\n        return 1\n"

	got := countChanges("tool.py", before, after, DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("default counts = %+v, want none", got)
	}

	f := DefaultFilters
	f.Whitespace = false
	f.Formatting = false
	got = countChanges("tool.py", before, after, f)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts without whitespace filter = %+v, want 1 modified", got)
	}
}

// Scenario: a JSON entry gets a trailing comma because a new entry was
// added after it.
// Goal: the comma is not a change, only the new entry counts; with the
// formatting filter off, the comma counts again.
func TestTrailingCommaIsFormatting(t *testing.T) {
	before := "{\n  \"typecheck\": \"tsc\"\n}\n"
	after := "{\n  \"typecheck\": \"tsc\",\n  \"apk\": \"build\"\n}\n"

	got := countChanges("package.json", before, after, DefaultFilters)
	if got != (diff.Counts{Added: 1}) {
		t.Errorf("counts = %+v, want 1 added", got)
	}

	f := DefaultFilters
	f.Formatting = false
	got = countChanges("package.json", before, after, f)
	if got != (diff.Counts{Modified: 1, Added: 1}) {
		t.Errorf("counts without formatting filter = %+v, want 1 modified, 1 added", got)
	}
}

// Scenario: Go call whose last argument gets a trailing comma, with a
// comment after it.
// Goal: not counted; a real change on the same line still is.
func TestTrailingCommaGo(t *testing.T) {
	before := "package a\n\nvar x = f(\n\ta, b\n)\n"
	after := "package a\n\nvar x = f(\n\ta, b, // note\n)\n"

	got := countChanges("a.go", before, after, DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}

	changed := "package a\n\nvar x = f(\n\ta, c,\n)\n"
	got = countChanges("a.go", before, changed, DefaultFilters)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts = %+v, want 1 modified", got)
	}
}

// Scenario: C preprocessor line, which Chroma tags as a comment type.
// Goal: changing an #include is a real change.
func TestCPreprocessorIsCode(t *testing.T) {
	got := countChanges("x.c", "#include <stdio.h>\n", "#include <stdlib.h>\n", DefaultFilters)
	if got != (diff.Counts{Modified: 1}) {
		t.Errorf("counts = %+v, want 1 modified", got)
	}
}

// Scenario: unknown file type, only spacing changed, and a blank line added.
// Goal: plain text handling still ignores whitespace.
func TestPlainTextWhitespace(t *testing.T) {
	got := countChanges("notes.unknownext", "a  b\nc\n", "a b\n\nc\n", DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}
}

// Scenario: file without a trailing newline and with Windows line endings.
// Goal: lines are split the same way, and an identical file has no changes.
func TestLineEndings(t *testing.T) {
	lines := Analyze("a.go", []byte("package a\r\n\r\nvar x = 1"), DefaultFilters)
	if len(lines) != 3 || lines[2].Text != "var x = 1" || lines[0].Text != "package a" {
		t.Fatalf("lines = %+v", lines)
	}

	got := countChanges("a.go", "package a\r\nvar x = 1\r\n", "package a\nvar x = 1\n", DefaultFilters)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}
}
