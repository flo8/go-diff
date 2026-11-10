package tui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
)

const (
	logLimit     = 1000                   // commits listed in the log tab
	loadDelay    = 120 * time.Millisecond // wait for the cursor to stop before loading
	maxCached    = 64                     // commits kept in memory
	hashColWidth = 9
	dateColWidth = 15
	authColWidth = 14
)

// focusArea is the pane of the log tab that receives the arrow keys.
type focusArea int

const (
	focusCommits focusArea = iota
	focusFiles
)

// logState is the state of the log tab: the commit history on the right,
// the selected commit's files on the left.
type logState struct {
	listed   bool // the history was read
	commits  []git.Commit
	err      error // reading the history failed
	selected int
	top      int
	focus    focusArea

	files   pane                  // the selected commit's files and viewer
	cache   map[string]*changeSet // loaded commits by hash
	loadErr map[string]error      // commits that failed to load
	seq     int                   // increases each time the selection moves
	loading string                // hash being loaded, "" when idle
}

// loadTick fires loadDelay after the selection moved. If the selection
// moved again meanwhile, its seq is outdated and it is dropped.
type loadTick struct {
	seq int
}

func (t loadTick) msg(time.Time) tea.Msg {
	return t
}

// commitLoaded carries a commit's files back from the background.
type commitLoaded struct {
	hash string
	set  *changeSet
	err  error
}

// loadJob reads and computes one commit's files. Its run method is a
// Bubble Tea command, so it runs off the main loop.
type loadJob struct {
	repo    *git.Repo
	hash    string
	filters semantic.Filters
}

func (j loadJob) run() tea.Msg {
	var rg git.Range
	var files []*review.File
	var err error

	rg, err = j.repo.ResolveRange(git.Options{Commit: j.hash})
	if err != nil {
		return commitLoaded{hash: j.hash, err: err}
	}

	files, err = review.Load(j.repo, rg)
	if err != nil {
		return commitLoaded{hash: j.hash, err: err}
	}

	review.ComputeAll(files, j.filters)
	return commitLoaded{hash: j.hash, set: newChangeSet(files, rg.String(), j.filters)}
}

// openLog reads the history the first time the log tab is shown.
func (m *Model) openLog() tea.Cmd {
	if m.log.listed {
		return nil
	}

	m.log.listed = true
	m.log.cache = map[string]*changeSet{}
	m.log.loadErr = map[string]error{}
	m.log.commits, m.log.err = m.repo.Log(logLimit)

	return m.selectCommit(0)
}

// selectCommit moves the selection to commit i and shows its files,
// right away when cached, after a short delay otherwise.
func (m *Model) selectCommit(i int) tea.Cmd {
	var hash string
	var set *changeSet

	if len(m.log.commits) == 0 {
		return nil
	}

	m.log.selected = min(max(i, 0), len(m.log.commits)-1)
	hash = m.log.commits[m.log.selected].Hash

	set = m.log.cache[hash]
	m.log.files = pane{set: set}
	if set != nil {
		set.recompute(m.filters)
		return nil
	}

	m.log.seq++
	return tea.Tick(loadDelay, loadTick{seq: m.log.seq}.msg)
}

// startLoad begins loading the selected commit, unless the selection
// moved since the tick was scheduled or the commit is already there.
func (m *Model) startLoad(t loadTick) tea.Cmd {
	var hash string

	if t.seq != m.log.seq || len(m.log.commits) == 0 {
		return nil
	}

	hash = m.log.commits[m.log.selected].Hash
	if m.log.cache[hash] != nil || m.log.loading == hash {
		return nil
	}

	m.log.loading = hash
	return loadJob{repo: m.repo, hash: hash, filters: m.filters}.run
}

// finishLoad stores a loaded commit and shows it if it is still selected.
func (m *Model) finishLoad(msg commitLoaded) tea.Cmd {
	m.log.loading = ""

	if msg.err != nil {
		m.log.loadErr[msg.hash] = msg.err
		return nil
	}

	// Keep memory bounded: forget everything when the cache is full.
	if len(m.log.cache) >= maxCached {
		m.log.cache = map[string]*changeSet{}
	}
	m.log.cache[msg.hash] = msg.set

	// The selection may have moved on while this commit was loading.
	if m.log.commits[m.log.selected].Hash != msg.hash {
		return m.selectCommit(m.log.selected)
	}

	msg.set.recompute(m.filters)
	m.log.files.set = msg.set
	return nil
}

// logKey handles keys on the log tab when no file is open.
func (m *Model) logKey(key string) tea.Cmd {
	if m.log.focus == focusFiles {
		m.logFilesKey(key)
		return nil
	}
	return m.logCommitsKey(key)
}

// logCommitsKey moves through the history.
func (m *Model) logCommitsKey(key string) tea.Cmd {
	var page int

	page = max(m.height-4, 1)

	switch key {
	case "up", "k":
		return m.selectCommit(m.log.selected - 1)
	case "down", "j":
		return m.selectCommit(m.log.selected + 1)
	case "pgup", "ctrl+b":
		return m.selectCommit(m.log.selected - page)
	case "pgdown", "ctrl+f", "space":
		return m.selectCommit(m.log.selected + page)
	case "home", "g":
		return m.selectCommit(0)
	case "end", "G":
		return m.selectCommit(len(m.log.commits) - 1)
	case "enter", "right", "l":
		if m.log.files.set != nil && len(m.log.files.set.list.visible) > 0 {
			m.log.focus = focusFiles
		}
	}

	return nil
}

// logFilesKey moves through the selected commit's files.
func (m *Model) logFilesKey(key string) {
	switch key {
	case "esc", "left", "h", "backspace":
		m.log.focus = focusCommits
	default:
		m.fileListKey(&m.log.files, key)
	}
}

// logView draws the log tab: header, files on the left, commits on the
// right, footer.
func (m *Model) logView() string {
	var height int
	var commitsWidth int
	var filesWidth int
	var left, right []string
	var lines []string
	var sep string

	height = max(m.height-2, 1)
	commitsWidth = min(max(m.width/2, 30), 90)
	filesWidth = max(m.width-commitsWidth-1, 1)

	left = m.logFilesPane(filesWidth, height)
	right = m.commitList(commitsWidth, height)
	sep = m.pal.Paint("│", m.pal.Border)

	lines = append(lines, m.headerLine(m.logTitle()))
	for i := 0; i < height; i++ {
		lines = append(lines, padTo(lineAt(left, i), filesWidth)+sep+padTo(lineAt(right, i), commitsWidth))
	}
	lines = append(lines, m.logFooter())

	return strings.Join(lines, "\n")
}

// logTitle names the selected commit in the header.
func (m *Model) logTitle() string {
	if len(m.log.commits) == 0 {
		return "history"
	}
	c := m.log.commits[m.log.selected]
	return c.Short + " " + c.Subject
}

// logFilesPane draws the selected commit's file table, or why there is
// none yet.
func (m *Model) logFilesPane(width, height int) []string {
	var hash string

	switch {
	case m.log.err != nil:
		return []string{m.pal.Paint("  "+m.log.err.Error(), m.pal.RemovedFg)}
	case len(m.log.commits) == 0:
		return []string{m.pal.Paint("  No commits yet.", m.pal.Muted)}
	}

	hash = m.log.commits[m.log.selected].Hash
	if err := m.log.loadErr[hash]; err != nil {
		return []string{m.pal.Paint("  "+err.Error(), m.pal.RemovedFg)}
	}
	if m.log.files.set == nil {
		return []string{m.pal.Paint("  Loading…", m.pal.Muted)}
	}

	return m.fileTable(m.log.files.set, width, height, m.log.focus == focusFiles)
}

// commitList draws the history: titles, a rule and one row per commit.
func (m *Model) commitList(width, height int) []string {
	var lines []string
	var rows int
	var titles string

	rows = max(height-2, 1)
	m.scrollCommits(rows)

	titles = "  " + padRight("Commit", hashColWidth) + padRight("Date", dateColWidth) +
		padRight("Author", authColWidth) + "Subject"
	lines = append(lines, m.pal.Style(m.pal.Muted, m.pal.Bg, true).Render(padTo(titles, width)))
	lines = append(lines, m.rule(width))

	for i := m.log.top; i < len(m.log.commits) && i < m.log.top+rows; i++ {
		lines = append(lines, m.commitRow(m.log.commits[i], i == m.log.selected, width))
	}

	return lines
}

// scrollCommits keeps the selected commit inside a window of rows.
func (m *Model) scrollCommits(rows int) {
	if m.log.selected < m.log.top {
		m.log.top = m.log.selected
	}
	if m.log.selected >= m.log.top+rows {
		m.log.top = m.log.selected - rows + 1
	}
}

// commitRow draws one commit: hash, date, author, subject.
func (m *Model) commitRow(c git.Commit, selected bool, width int) string {
	var focused bool
	var bg color.Color
	var marker string
	var b strings.Builder

	focused = selected && m.log.focus == focusCommits
	bg = m.pal.Bg
	marker = "  "
	if selected {
		marker = "▸ "
	}
	if focused {
		bg = m.pal.Selection
	}

	b.WriteString(m.pal.Style(m.pal.Header, bg, true).Render(marker))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, bg, false).Render(padRight(c.Short, hashColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Date, dateColWidth-1), dateColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Author, authColWidth-1), authColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, bg, focused).Render(c.Subject))

	return m.padLine(b.String(), bg, width)
}

// logFooter lists the keys of the focused pane.
func (m *Model) logFooter() string {
	if m.log.focus == focusFiles {
		return m.keyHints([][2]string{
			{"↑↓", "select file"}, {"⏎", "open"}, {"esc/←", "commits"},
			{"o", "sort: " + sortName(m.log.files.set.list)}, {"tab", "changes"}, {"?", "help"}, {"q", "quit"},
		})
	}
	return m.keyHints([][2]string{
		{"↑↓", "select commit"}, {"⏎/→", "files"}, {"tab", "changes"},
		{"t", "theme: " + m.pal.Theme.Name}, {"?", "help"}, {"q", "quit"},
	})
}

// lineAt returns lines[i], or "" past the end.
func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}
