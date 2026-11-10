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
	logLimit     = 1000                   // commits listed in the history
	loadDelay    = 120 * time.Millisecond // wait for the cursor to stop before loading
	maxCached    = 64                     // commits kept in memory
	hashColWidth = 9
	dateColWidth = 15
	authColWidth = 14
)

// history is the list under the file table. Entry 0 is "current", the
// comparison given on the command line (by default the uncommitted
// changes); entry i > 0 is commits[i-1], compared with its parent.
type history struct {
	commits  []git.Commit
	err      error // reading the history failed
	selected int   // entry index
	top      int

	cache   map[string]*changeSet // loaded commits by hash
	loadErr map[string]error      // commits that failed to load
	seq     int                   // increases each time the selection moves
	loading string                // hash being loaded, "" when idle
}

// entries is the number of rows: "current" plus the commits.
func (h *history) entries() int {
	return len(h.commits) + 1
}

// selectedCommit returns the selected commit, or false on "current".
func (h *history) selectedCommit() (git.Commit, bool) {
	if h.selected == 0 {
		return git.Commit{}, false
	}
	return h.commits[h.selected-1], true
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

// selectEntry moves the history selection to entry i and shows its files:
// "current" and cached commits right away, other commits after a short
// delay so that scrolling through the list does not load every commit.
func (m *Model) selectEntry(i int) tea.Cmd {
	var commit git.Commit
	var isCommit bool
	var set *changeSet

	m.hist.selected = min(max(i, 0), m.hist.entries()-1)
	commit, isCommit = m.hist.selectedCommit()

	set = m.current
	if isCommit {
		set = m.hist.cache[commit.Hash]
	}

	m.files = pane{set: set}
	if set != nil {
		set.recompute(m.filters)
		return nil
	}

	m.hist.seq++
	return tea.Tick(loadDelay, loadTick{seq: m.hist.seq}.msg)
}

// startLoad begins loading the selected commit, unless the selection
// moved since the tick was scheduled or the commit is already there.
func (m *Model) startLoad(t loadTick) tea.Cmd {
	var commit git.Commit
	var isCommit bool

	commit, isCommit = m.hist.selectedCommit()
	if t.seq != m.hist.seq || !isCommit {
		return nil
	}
	if m.hist.cache[commit.Hash] != nil || m.hist.loading == commit.Hash {
		return nil
	}

	m.hist.loading = commit.Hash
	return loadJob{repo: m.repo, hash: commit.Hash, filters: m.filters}.run
}

// finishLoad stores a loaded commit and shows it if it is still selected.
func (m *Model) finishLoad(msg commitLoaded) tea.Cmd {
	var commit git.Commit
	var isCommit bool

	m.hist.loading = ""

	if msg.err != nil {
		m.hist.loadErr[msg.hash] = msg.err
		return nil
	}

	// Keep memory bounded: forget everything when the cache is full.
	if len(m.hist.cache) >= maxCached {
		m.hist.cache = map[string]*changeSet{}
	}
	m.hist.cache[msg.hash] = msg.set

	// The selection may have moved on while this commit was loading.
	commit, isCommit = m.hist.selectedCommit()
	if !isCommit || commit.Hash != msg.hash {
		return m.selectEntry(m.hist.selected)
	}

	msg.set.recompute(m.filters)
	m.files.set = msg.set
	return nil
}

// historyKey moves through the history when it has the focus.
func (m *Model) historyKey(key string) tea.Cmd {
	var page int

	page = max(m.historyHeight()-1, 1)

	switch key {
	case "up", "k":
		return m.selectEntry(m.hist.selected - 1)
	case "down", "j":
		return m.selectEntry(m.hist.selected + 1)
	case "pgup", "ctrl+b":
		return m.selectEntry(m.hist.selected - page)
	case "pgdown", "ctrl+f", "space":
		return m.selectEntry(m.hist.selected + page)
	case "home", "g":
		return m.selectEntry(0)
	case "end", "G":
		return m.selectEntry(m.hist.entries() - 1)
	case "enter", "esc", "right", "l":
		m.focus = focusFiles
	}

	return nil
}

// historyHeight is the number of rows of the history panel, rule
// included: a third of the screen, between 5 and 12 rows.
func (m *Model) historyHeight() int {
	return min(max((m.height-2)/3, 5), 12)
}

// historyPanel draws the history in height lines: a rule with a title,
// then one row per entry.
func (m *Model) historyPanel(height int) []string {
	var lines []string
	var rows int

	rows = max(height-1, 1)
	m.scrollHistory(rows)

	lines = append(lines, m.titledRule("History", m.width))

	if m.hist.err != nil {
		lines = append(lines, m.pal.Paint("  "+m.hist.err.Error(), m.pal.RemovedFg))
	}

	for i := m.hist.top; i < m.hist.entries() && i < m.hist.top+rows; i++ {
		lines = append(lines, m.historyRow(i))
	}

	return lines
}

// scrollHistory keeps the selected entry inside a window of rows.
func (m *Model) scrollHistory(rows int) {
	if m.hist.selected < m.hist.top {
		m.hist.top = m.hist.selected
	}
	if m.hist.selected >= m.hist.top+rows {
		m.hist.top = m.hist.selected - rows + 1
	}
}

// historyRow draws one entry: "current" or a commit's hash, date,
// author and subject.
func (m *Model) historyRow(i int) string {
	var selected bool
	var focused bool
	var bg color.Color
	var marker string
	var c git.Commit
	var b strings.Builder

	selected = i == m.hist.selected
	focused = selected && m.focus == focusHistory
	bg = m.pal.Bg
	marker = "  "
	if selected {
		marker = "▸ "
	}
	if focused {
		bg = m.pal.Selection
	}

	// "current" stands in for the hash of the comparison from the
	// command line.
	c = git.Commit{Short: "current", Subject: m.currentSubject()}
	if i > 0 {
		c = m.hist.commits[i-1]
	}

	b.WriteString(m.pal.Style(m.pal.Header, bg, true).Render(marker))
	b.WriteString(m.pal.Style(m.pal.ModifiedFg, bg, i == 0).Render(padRight(c.Short, hashColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Date, dateColWidth-1), dateColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Author, authColWidth-1), authColWidth)))
	b.WriteString(m.pal.Style(m.pal.Fg, bg, focused).Render(c.Subject))

	return m.padLine(b.String(), bg, m.width)
}

// currentSubject describes the "current" entry.
func (m *Model) currentSubject() string {
	if len(m.current.list.visible) == 0 {
		return m.current.rangeName + "  (no changes)"
	}
	return m.current.rangeName
}

// titledRule draws "── Title ─────" across width columns.
func (m *Model) titledRule(title string, width int) string {
	var head string

	head = "── " + title + " "
	return m.pal.Paint(head, m.pal.Muted) + m.pal.Paint(strings.Repeat("─", max(width-len([]rune(head)), 0)), m.pal.Border)
}
