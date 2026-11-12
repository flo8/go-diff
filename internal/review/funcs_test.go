package review

import (
	"testing"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/semantic"
)

// computeGo builds a modified Go file from two versions and computes it
// with the default filters.
func computeGo(path, old, new string) *File {
	var file *File

	file = &File{
		Change: git.Change{OldPath: path, NewPath: path, Status: git.Modified},
		Old:    []byte(old),
		New:    []byte(new),
	}
	file.Compute(semantic.DefaultFilters)

	return file
}

const funcsBefore = `package main

type T struct{ n int }

func A() int {
	return 1
}

// B is documented.
func B() int {
	return 2
}

func C() int {
	x := 3
	return x
}

func (t *T) M() int {
	return t.n
}

func D() {}
`

// Scenario: one edit of each kind in a Go file. A's body changes, B only
// gets a new comment, C gets a blank line and new spacing, the struct
// type changes, method T.M changes, D is deleted at the end and E is
// added at the top.
// Goal: only functions whose code changed count: A, T.M, D and E.
func TestChangedFuncsIgnoreCommentsAndFormatting(t *testing.T) {
	after := `package main

type T struct{ n, m int }

func E() {}

func A() int {
	return 10
}

// B is documented, and now says more.
func B() int {
	return 2 // still two
}

func C() int {

	x:=3
	return x
}

func (t *T) M() int {
	return t.n + t.m
}
`
	file := computeGo("main.go", funcsBefore, after)
	if file.Funcs != 4 {
		t.Errorf("Funcs = %d, want 4 (A, T.M, D, E)", file.Funcs)
	}
}

// Scenario: a function is renamed and nothing else changes.
// Goal: it counts as one changed function, not one removed and one added.
func TestChangedFuncsRenameCountsOnce(t *testing.T) {
	after := `package main

type T struct{ n int }

func A2() int {
	return 1
}

// B is documented.
func B() int {
	return 2
}

func C() int {
	x := 3
	return x
}

func (t *T) M() int {
	return t.n
}

func D() {}
`
	file := computeGo("main.go", funcsBefore, after)
	if file.Funcs != 1 {
		t.Errorf("Funcs = %d, want 1", file.Funcs)
	}
}

// Scenario: a non-Go file with a real change.
// Goal: no functions are counted.
func TestChangedFuncsNonGoFile(t *testing.T) {
	file := computeGo("notes.txt", "func A() {}\n", "func B() {}\n")
	if file.Funcs != 0 {
		t.Errorf("Funcs = %d, want 0", file.Funcs)
	}
}
