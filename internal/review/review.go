// Package review loads the changed files of a comparison and computes
// their meaningful changes under a set of filters.
package review

import (
	"bytes"
	"runtime"
	"sync"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/semantic"
)

// binarySniffLen is how many leading bytes are checked for a NUL byte,
// the same heuristic git uses to call a file binary.
const binarySniffLen = 8000

// File is one changed file with both of its versions.
type File struct {
	Change git.Change
	Old    []byte // nil when the file does not exist in the base
	New    []byte // nil when the file does not exist in the target
	Binary bool

	// Results of the last Compute.
	OldLines []semantic.Line
	NewLines []semantic.Line
	Rows     []diff.Row
	Counts   diff.Counts
	Funcs    int // Go functions with a changed line; 0 for other files
}

// Load lists the changes in rg and reads both versions of every file.
func Load(repo *git.Repo, rg git.Range) ([]*File, error) {
	var changes []git.Change
	var contents []git.Contents
	var files []*File
	var err error

	changes, err = repo.Changes(rg)
	if err != nil {
		return nil, err
	}

	contents, err = repo.ReadContents(rg, changes)
	if err != nil {
		return nil, err
	}

	files = make([]*File, len(changes))
	for i, c := range changes {
		files[i] = &File{
			Change: c,
			Old:    contents[i].Old,
			New:    contents[i].New,
			Binary: isBinary(contents[i].Old) || isBinary(contents[i].New),
		}
	}

	return files, nil
}

// ComputeAll recomputes every file under the filters, several files at
// a time: files do not depend on each other and the work is CPU-bound.
func ComputeAll(files []*File, f semantic.Filters) {
	var work chan *File
	var wg sync.WaitGroup
	var workers int

	workers = min(runtime.GOMAXPROCS(0), len(files))
	work = make(chan *File)

	wg.Add(workers)
	for range workers {
		go computeFiles(work, f, &wg)
	}

	for _, file := range files {
		work <- file
	}
	close(work)
	wg.Wait()
}

// computeFiles computes the files it receives until work is closed.
func computeFiles(work <-chan *File, f semantic.Filters, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range work {
		file.Compute(f)
	}
}

// Compute analyzes both versions, aligns them and counts the changes.
// Binary files are not compared.
func (file *File) Compute(f semantic.Filters) {
	var oldPath, newPath string
	var oldFuncs, newFuncs []semantic.Func

	if file.Binary {
		return
	}

	// Each version is analyzed under its own name (they differ for renames).
	oldPath = file.Change.OldPath
	newPath = file.Change.NewPath
	file.OldLines = semantic.Analyze(oldPath, file.Old, f)
	file.NewLines = semantic.Analyze(newPath, file.New, f)

	file.Rows = diff.Align(side(file.OldLines), side(file.NewLines))
	file.Counts = diff.Count(file.Rows)

	// Skip parsing when every change was filtered out.
	file.Funcs = 0
	if file.Counts.Total() > 0 {
		oldFuncs = semantic.GoFuncs(oldPath, file.Old)
		newFuncs = semantic.GoFuncs(newPath, file.New)
		file.Funcs = countChangedFuncs(file.Rows, oldFuncs, newFuncs)
	}
}

// Meaningful reports whether the file belongs in the summary: it has
// counted changes, is binary, or was renamed (a move matters on its own).
func (file *File) Meaningful() bool {
	if file.Binary {
		return file.oldDiffersFromNew()
	}
	return file.Counts.Total() > 0 || file.Change.Status == git.Renamed
}

// oldDiffersFromNew is true when the bytes changed at all.
func (file *File) oldDiffersFromNew() bool {
	return !bytes.Equal(file.Old, file.New)
}

// side converts analyzed lines to what the aligner needs.
func side(lines []semantic.Line) diff.Side {
	var s diff.Side

	s.Keys = make([]string, len(lines))
	s.Ignored = make([]bool, len(lines))
	for i, l := range lines {
		s.Keys[i] = l.Key
		s.Ignored[i] = l.Ignored
	}

	return s
}

// isBinary reports whether data looks like a binary file.
func isBinary(data []byte) bool {
	var head []byte

	head = data[:min(len(data), binarySniffLen)]
	return bytes.IndexByte(head, 0) >= 0
}
