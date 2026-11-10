package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Contents holds both versions of one changed file.
// A nil slice means the file does not exist on that side.
type Contents struct {
	Old []byte
	New []byte
}

// blobRequest asks for one file version stored in git.
type blobRequest struct {
	spec string  // object name for cat-file, like "HEAD:main.go" or ":main.go"
	dest *[]byte // where to store the bytes
}

// ReadContents reads the old and new versions of every change.
// Versions stored in git are read with a single "git cat-file --batch"
// process; working tree versions are read from disk.
func (r *Repo) ReadContents(rg Range, changes []Change) ([]Contents, error) {
	var contents []Contents
	var requests []blobRequest
	var err error

	contents = make([]Contents, len(changes))

	for i, c := range changes {
		if c.OldPath != "" {
			requests, err = r.readSide(rg.Base, c.OldPath, &contents[i].Old, requests)
			if err != nil {
				return nil, err
			}
		}
		if c.NewPath != "" {
			requests, err = r.readSide(rg.Target, c.NewPath, &contents[i].New, requests)
			if err != nil {
				return nil, err
			}
		}
	}

	err = r.catFile(requests)
	if err != nil {
		return nil, err
	}

	return contents, nil
}

// readSide reads a working tree file right away, or queues a request
// for a file stored in git.
func (r *Repo) readSide(side Side, path string, dest *[]byte, requests []blobRequest) ([]blobRequest, error) {
	var data []byte
	var err error

	switch side.Kind {
	case FromWorktree:
		data, err = r.readWorktree(path)
		if err != nil {
			return nil, err
		}
		*dest = data
	case FromIndex:
		requests = append(requests, blobRequest{spec: ":" + path, dest: dest})
	case FromRev:
		requests = append(requests, blobRequest{spec: side.Rev + ":" + path, dest: dest})
	}

	return requests, nil
}

// readWorktree reads a file from disk the way git stores it:
// a symbolic link is read as its target path.
func (r *Repo) readWorktree(path string) ([]byte, error) {
	var full string
	var info os.FileInfo
	var target string
	var err error

	full = filepath.Join(r.Root, path)
	info, err = os.Lstat(full)
	if err != nil {
		return nil, err
	}

	// Symbolic links are stored by git as the link target.
	if info.Mode()&os.ModeSymlink != 0 {
		target, err = os.Readlink(full)
		if err != nil {
			return nil, err
		}
		return []byte(target), nil
	}

	// Directories show up for submodules; there is no text to compare.
	if info.IsDir() {
		return []byte{}, nil
	}

	return os.ReadFile(full)
}

// catFile runs one "git cat-file --batch" for all requests and stores
// each object's bytes in its destination.
func (r *Repo) catFile(requests []blobRequest) error {
	var cmd *exec.Cmd
	var input strings.Builder
	var out []byte
	var reader *bufio.Reader
	var err error

	if len(requests) == 0 {
		return nil
	}

	// One object name per line on stdin.
	for _, req := range requests {
		input.WriteString(req.spec)
		input.WriteByte('\n')
	}

	cmd = exec.Command("git", "--no-optional-locks", "cat-file", "--batch")
	cmd.Dir = r.Root
	cmd.Stdin = strings.NewReader(input.String())
	out, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("git cat-file: %w", err)
	}

	// The answers come back in the same order as the requests.
	reader = bufio.NewReader(bytes.NewReader(out))
	for _, req := range requests {
		*req.dest, err = readBatchObject(reader)
		if err != nil {
			return fmt.Errorf("reading %s: %w", req.spec, err)
		}
	}

	return nil
}

// readBatchObject reads one answer from "git cat-file --batch":
//
//	<oid> <type> <size>\n<content>\n
//
// or "<name> missing\n" when the object does not exist.
func readBatchObject(reader *bufio.Reader) ([]byte, error) {
	var header string
	var fields []string
	var size int
	var data []byte
	var err error

	header, err = reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	// A missing object is treated as an absent file.
	header = strings.TrimSuffix(header, "\n")
	if strings.HasSuffix(header, " missing") {
		return nil, nil
	}

	fields = strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("unexpected cat-file header %q", header)
	}

	size, err = strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("bad size in cat-file header %q", header)
	}

	// Content, then a trailing newline that is not part of the object.
	data = make([]byte, size)
	_, err = io.ReadFull(reader, data)
	if err != nil {
		return nil, err
	}
	_, err = reader.Discard(1)
	if err != nil {
		return nil, err
	}

	return data, nil
}
