# godiff

A terminal diff viewer for code review. It lists the files that have
meaningful changes, counts removed, added and modified lines, and shows a
side-by-side diff with the changed words highlighted inside each line.

Whitespace, formatting and comment edits are filtered out by default.
godiff only reads the repository: it never changes files or the index.

## Build

From a clone:

    go install ./cmd/godiff

From anywhere:

    go install github.com/flo8/go-diff/cmd/godiff@latest

Both put `godiff` in `$(go env GOPATH)/bin` (usually `~/go/bin`).

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
| `f` | formatting | Lines are compared token by token, so `a+b` equals `a + b`. On by default. |
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

## Keys

Press `?` in the app for the full list.

| Key | Summary | Diff viewer |
|---|---|---|
| `↑ ↓` / `j k` | select file | scroll |
| `Enter` | open file | |
| `Esc` | | back to summary |
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
