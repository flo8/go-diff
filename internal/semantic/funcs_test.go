package semantic

import (
	"reflect"
	"testing"
)

// Scenario: a Go file with a plain function, methods on a pointer and a
// generic receiver, and a syntax error at the end.
// Goal: every function is found with its receiver-qualified name and
// its 0-based line range, despite the error.
func TestGoFuncs(t *testing.T) {
	src := `package main

// Run runs.
func Run() {
}

func (s *Set[K]) Add(k K) {}

func (m Map[K, V]) Get() {
	return
}

func Broken( {
`
	got := GoFuncs("main.go", []byte(src))
	want := []Func{
		{Name: "Run", FirstLine: 3, LastLine: 4},
		{Name: "Set.Add", FirstLine: 6, LastLine: 6},
		{Name: "Map.Get", FirstLine: 8, LastLine: 10},
	}
	if len(got) < len(want) || !reflect.DeepEqual(got[:len(want)], want) {
		t.Errorf("GoFuncs = %+v, want %+v first", got, want)
	}
}

// Scenario: a file that is not Go.
// Goal: no functions.
func TestGoFuncsNonGo(t *testing.T) {
	if got := GoFuncs("main.py", []byte("def f():\n  pass\n")); got != nil {
		t.Errorf("GoFuncs = %+v, want nil", got)
	}
}
