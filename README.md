# godiff

A terminal diff viewer for code review. It lists the files that have
meaningful changes, counts removed, added and modified lines, and shows a
side-by-side diff with the changed words highlighted inside each line.

Whitespace, formatting and comment edits are filtered out by default.
godiff only reads the repository: it never changes files or the index.

## Build

From a clone:

    make install

From anywhere:

    go install github.com/flo8/go-diff/cmd/godiff@latest

Both put `godiff` in `$(go env GOPATH)/bin` (usually `~/go/bin`), so it
runs from any folder if that directory is on your `PATH`. Run
`make install` again after pulling or changing the code.

## Development

| Target | What it does |
|---|---|
| `make` / `make check` | everything below that should pass before a commit: `fmt-check`, `vet`, `lint`, `test`, `vulncheck` |
| `make build` | build `bin/godiff` |
| `make install` | `go install ./cmd/godiff` |
| `make test` | run the tests (under a second) |
| `make race` | run the tests with the race detector |
| `make vet` | `go vet ./...` |
| `make lint` | staticcheck (S1021 is turned off in `staticcheck.conf`: variables are declared at the top of functions on purpose) |
| `make vulncheck` | govulncheck: known vulnerabilities in the code and its dependencies |
| `make fmt` / `make fmt-check` | format with gofmt / fail if a file is not formatted |
| `make tidy` | `go mod tidy` |
| `make clean` | remove `bin/` |

staticcheck and govulncheck run with `go run tool@latest`: nothing to
install, and `go.mod` does not change.

## Usage

    godiff                  working tree vs HEAD (staged + unstaged + untracked)
    godiff --staged         index vs HEAD
    godiff --unstaged       working tree vs index
    godiff --commit REV     REV vs its parent
    godiff BASE             working tree vs BASE
    godiff BASE TARGET      BASE vs TARGET (commits or branches)
    godiff A..B             same as godiff A B
    godiff A...B            B vs the point where it forked from A (what a pull request shows)

Flags: `--theme NAME`, `--config PATH`.

## What the filters do

Each line gets a comparison key. Two lines with the same key are equal, and
a line whose key is empty (comment-only, blank) is left out of the
comparison entirely, so adding or removing it is never a change.

| Key | Filter | Effect |
|---|---|---|
| `c` | comments | Comment tokens are removed before comparing. On by default. |
| `w` | whitespace | Indentation, runs of spaces and blank lines are ignored. On by default. |
| `f` | formatting | Lines are compared token by token, so `a+b` equals `a + b`, and a trailing comma is ignored, so `"tsc"` equals `"tsc",`. On by default. |
| `i` | imports | Go import declarations are ignored. Off by default. |

Go files are read with `go/scanner` (comments, tokens) and `go/parser`
(import blocks). Both accept code that does not compile. Other languages
use Chroma's lexers to find comments and tokens. Unknown file types are
compared as plain text, with only the whitespace filter applying.

Counting: inside each run of changed lines, the k-th removed line is
paired with the k-th added line and counted as one *modified* line. The
rest are *removed* or *added*.

Word highlighting splits lines into identifiers, numbers, whitespace and
single punctuation characters, so `timeout := 500 * time.Millisecond` →
`timeout := 1000 * time.Millisecond` highlights only `500` and `1000`.
Unknown file types are compared character by character.

## Tabs

godiff has two tabs, like lazygit. Each keeps its place when you switch.

| Key | Tab |
|---|---|
| `1` | **Changes**: the comparison given on the command line |
| `2` | **Log**: the commit history on the right; the selected commit's files (against its parent) on the left |
| `tab` | switch between the two |

In the log, `↑ ↓` moves through commits, `Enter` or `→` moves to the
commit's files, and `Esc` or `←` goes back to the commits. A commit's
files load in the background once the cursor stops on it. The last 1000
commits from HEAD are listed.

## Keys

Press `?` in the app for the full list.

| Key | File table | Diff viewer |
|---|---|---|
| `↑ ↓` / `j k` | select file | scroll |
| `Enter` | open file | |
| `Esc` | (log) back to commits | back to the file table |
| `n` / `N` | | next / previous change |
| `← →` / `h l` | | scroll left / right |
| `s` | | side-by-side or unified |
| `[` / `]` | | previous / next file |
| `o` | sort by name or by changes | |
| `c w f i` | toggle filters | toggle filters |
| `t` | next theme | next theme |
| `q` | quit | quit |

## Configuration

Optional, in `~/.config/godiff/config.json`:

```json
{
  "theme": "light",
  "filters": {"comments": true, "whitespace": true, "formatting": true, "imports": false},
  "themes": [
    {"name": "mine", "base": "dark", "added_bg": "#123b22", "syntax": {"keyword": "#ff79c6"}}
  ]
}
```

Built-in themes: `dark`, `light`, `high-contrast`, and `ansi` (uses the
terminal's own 16 colors, so it follows the terminal's color scheme).
Without a configured theme, godiff asks the terminal for its background
color and picks `dark` or `light`.

A custom theme starts from `base` and overrides any field. Colors are
`"#rrggbb"`, a palette index `"0"` to `"255"`, or `""` for the terminal
default. True colors are converted down on terminals that lack them. The
field names are in `internal/theme/theme.go`.

## Layout

    cmd/godiff         command line, config file
    internal/git       git commands: changed files, file contents (one cat-file process)
    internal/semantic  per-language tokenizers and comparison keys
    internal/diff      line matching (Myers, patience-style anchors), pairing, word diff
    internal/review    ties the above together for each file
    internal/syntax    syntax highlighting classes from Chroma
    internal/theme     theme definitions and custom themes
    internal/render    drawing side-by-side and unified lines
    internal/tui       Bubble Tea screens and keys

## Limits

- Files over 1 MiB are compared as plain text, without syntax colors.
- Lines moved to another place show up as removed and added.
- Keys are not configurable yet.
- A change that only re-wraps a statement over more lines (for example,
  splitting a call's arguments over several lines) is still counted,
  because comparison is line by line.
