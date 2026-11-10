# godiff

A terminal diff viewer for code review. It lists the files that have
meaningful changes, counts removed, added and modified lines, and shows a
side-by-side diff with the changed words highlighted inside each line.

Whitespace, formatting and comment edits are filtered out by default.
godiff only reads the repository, except when you press one of the git
keys (stage, commit, pull, push): it never changes files on disk.

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
| `F c` | comments | Comment tokens are removed before comparing. On by default. |
| `F w` | whitespace | Indentation, runs of spaces and blank lines are ignored. On by default. |
| `F f` | formatting | Lines are compared token by token, so `a+b` equals `a + b`, and a trailing comma is ignored, so `"tsc"` equals `"tsc",`. On by default. |
| `F i` | imports | Go import declarations are ignored. Off by default. |

Press `F`, then the filter's key.

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

## History

Under the file table, a history panel lists the commits from HEAD (the
last 1000). Its first row, `current`, is the comparison given on the
command line: by default the uncommitted changes. Selecting a commit
shows its files, compared with its parent, in the table above.

| Key | Action |
|---|---|
| `tab` | move between the file table and the history |
| `L` | show / hide the history panel |

Each row shows the commit's number of files and its removed, added and
modified totals, with the same filters as the file table. Commits load
in the background, one at a time: the selected commit first, then the
other rows on screen. Rows show `·` until their totals are known.

The panel's title shows the branch, its upstream, and how many commits
it is ahead (`↑`) and behind (`↓`) as of the last fetch.

Every 5 seconds godiff reads the working tree, the status and the
history again, so changes made outside it (an edit, a commit or fetch
from the shell) show up on their own. It waits while a file is open in
the diff viewer or a commit message is being typed.

## Commit, pull, push

| Key | Action |
|---|---|
| `space` | stage or unstage the selected file |
| `a` | stage everything, or unstage everything when all is staged |
| `c` | commit the staged files with a one-line message (Enter commits, Esc cancels) |
| `C` | commit with your editor (`$EDITOR`), for a full message |
| `p` | `git pull --ff-only`: fast-forwards, or fails if the branches have diverged |
| `P` | `git push`; a branch without upstream goes to `origin` and gets it set |

Staging works on `current` when godiff runs without arguments (HEAD vs
working tree). The file table then shows `●` staged, `◐` partly staged
and `○` not staged. Commit, pull and push work from anywhere.

Commit, pull and push run in the background with a spinner and, when git
reports one, a progress bar. The result stays in the footer: a success
for a few seconds, an error (with git's output) until the next key.
Hooks run as usual. godiff never merges, rebases or force-pushes.

Git cannot ask for anything while the TUI owns the terminal, so password
prompts are turned off: SSH keys from the agent work, a key that needs
its passphrase typed fails with git's message. If committing needs the
terminal (a GPG pinentry in the terminal, for example), use `C`.

## Keys

Press `?` in the app for the full list.

| Key | File table | Diff viewer |
|---|---|---|
| `↑ ↓` / `j k` | select file | scroll |
| `Enter` | open file | |
| `Esc` | | back to the file table |
| `n` / `N` | | next / previous change |
| `← →` / `h l` | | scroll left / right |
| `s` | | side-by-side or unified |
| `[` / `]` | | previous / next file |
| `o` | sort by changed lines (default) or by name | |
| `O` | group by go files, tests and other files (off by default) | |
| `F` then `c w f i` | toggle filters | toggle filters |
| `space` / `a` | stage file / all | |
| `c` / `C` | commit | commit |
| `p` / `P` | pull / push | pull / push |
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
    internal/git       git commands: changed files, file contents (one cat-file process);
                       actions.go has the commands that change the repository
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
