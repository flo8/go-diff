package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/diff"
	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/syntax"
)

const (
	logLimit      = 1000                   // commits listed in the history
	loadDelay     = 120 * time.Millisecond // wait for the cursor to stop before loading
	maxCached     = 64                     // commits kept in memory
	maxHistory    = 12                     // most rows of the history panel, rule included
	maxLoads      = maxHistory             // commits loading at the same time: a full panel
	hashColWidth  = 9
	dateColWidth  = 15
	authColWidth  = 14
	countColWidth = 6  // removed, added, modified, funcs and packages totals
	filesColWidth = 10 // "12 files"
)

// history is the list under the file table. Entry 0 is "current", the
// comparison given on the command line (by default the uncommitted
// changes); entry i > 0 is commits[i-1], compared with its parent.
type history struct {
	commits  []git.Commit
	unpushed map[string]bool // hashes of commits not pushed yet
	err      error           // reading the history failed
	selected int             // entry index
	top      int

	cache   map[string]*changeSet // loaded commits by hash
	loadErr map[string]error      // commits that failed to load
	seq     int                   // increases each time the selection moves
	loading map[string]bool       // hashes being loaded
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
// delay so that holding a key down does not start a load for each row.
func (m *Model) selectEntry(i int) tea.Cmd {
	var commit git.Commit
	var isCommit bool
	var set *changeSet

	m.hist.selected = min(max(i, 0), m.hist.entries()-1)
	m.scrollHistory(m.historyRows())
	commit, isCommit = m.hist.selectedCommit()

	set = m.current
	if isCommit {
		set = m.hist.cache[commit.Hash]
	}

	// Shown right away; rows that scrolled into view may need loading.
	m.files = pane{set: set}
	if set != nil {
		set.recompute(m.filters)
		m.applyView(set)
		return m.nextLoad()
	}

	m.hist.seq++
	return tea.Tick(loadDelay, loadTick{seq: m.hist.seq}.msg)
}

// startLoad runs when the cursor stopped on an unloaded commit, unless
// it moved again since.
func (m *Model) startLoad(t loadTick) tea.Cmd {
	if t.seq != m.hist.seq {
		return nil
	}
	return m.nextLoad()
}

// nextLoad starts loading commits, up to maxLoads at the same time: the
// selected commit first, then the visible rows from the top, so that
// every row on screen gets its counts.
func (m *Model) nextLoad() tea.Cmd {
	var cmds []tea.Cmd

	if m.repo == nil {
		return nil
	}

	for _, hash := range m.wantedCommits(maxLoads - len(m.hist.loading)) {
		m.hist.loading[hash] = true
		cmds = append(cmds, loadJob{repo: m.repo, hash: hash, filters: m.filters}.run)
	}
	return tea.Batch(cmds...)
}

// wantedCommits returns up to n commits to load next, in order: the
// selected one, then the visible ones. Commits already loaded, loading
// or failed are skipped.
func (m *Model) wantedCommits(n int) []string {
	var candidates []string
	var wanted []string

	commit, isCommit := m.hist.selectedCommit()
	if isCommit {
		candidates = append(candidates, commit.Hash)
	}
	candidates = append(candidates, m.visibleCommits()...)

	for _, hash := range candidates {
		if len(wanted) >= n {
			break
		}
		if m.hist.cache[hash] != nil || m.hist.loadErr[hash] != nil || m.hist.loading[hash] {
			continue
		}
		if !slices.Contains(wanted, hash) {
			wanted = append(wanted, hash)
		}
	}
	return wanted
}

// visibleCommits returns the hashes of the commits on screen.
func (m *Model) visibleCommits() []string {
	var hashes []string

	for i := max(m.hist.top, 1); i < m.hist.entries() && i < m.hist.top+m.historyRows(); i++ {
		hashes = append(hashes, m.hist.commits[i-1].Hash)
	}
	return hashes
}

// finishLoad stores a loaded commit, shows it if it is selected, and
// starts the next loads.
func (m *Model) finishLoad(msg commitLoaded) tea.Cmd {
	var commit git.Commit
	var isCommit bool

	delete(m.hist.loading, msg.hash)

	if msg.err != nil {
		m.hist.loadErr[msg.hash] = msg.err
		return m.nextLoad()
	}

	if len(m.hist.cache) >= maxCached {
		m.evictHidden()
	}
	m.hist.cache[msg.hash] = msg.set
	msg.set.recompute(m.filters)

	commit, isCommit = m.hist.selectedCommit()
	if isCommit && commit.Hash == msg.hash {
		m.applyView(msg.set)
		m.files.set = msg.set
	}

	return m.nextLoad()
}

// evictHidden keeps memory bounded: it forgets every cached commit that
// is neither visible nor selected.
func (m *Model) evictHidden() {
	var keep map[string]*changeSet

	keep = map[string]*changeSet{}
	for _, hash := range m.visibleCommits() {
		if set := m.hist.cache[hash]; set != nil {
			keep[hash] = set
		}
	}

	commit, isCommit := m.hist.selectedCommit()
	if set := m.hist.cache[commit.Hash]; isCommit && set != nil {
		keep[commit.Hash] = set
	}

	m.hist.cache = keep
}

// historyRows is the number of entries the history panel shows.
func (m *Model) historyRows() int {
	return max(m.historyHeight()-1, 1)
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
// included: a third of the screen, between 5 and maxHistory rows.
func (m *Model) historyHeight() int {
	return min(max((m.height-2)/3, 5), maxHistory)
}

// historyPanel draws the history in height lines: a rule with a title,
// then one row per entry.
func (m *Model) historyPanel(height int) []string {
	var lines []string
	var rows int

	rows = max(height-1, 1)
	m.scrollHistory(rows)

	lines = append(lines, m.titledRule("History"+m.branchText(), m.width))

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
	b.WriteString(m.pal.Style(m.hashColor(i, c), bg, i == 0).Render(padRight(c.Short, hashColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Date, dateColWidth-1), dateColWidth)))
	b.WriteString(m.pal.Style(m.pal.Muted, bg, false).Render(padRight(truncate(c.Author, authColWidth-1), authColWidth)))
	b.WriteString(m.entryCounts(m.entrySet(i), bg))
	b.WriteString(m.pal.Style(m.pal.Fg, bg, focused).Render("  " + c.Subject))

	return m.padLine(b.String(), bg, m.width)
}

// hashColor colors the hash of entry i: pink for work not pushed yet
// ("current" with changes, unpushed commits), the theme's type color
// (purple in dark) for pushed commits, and grey for "current" when
// there is nothing to commit. "current" counts files whose changes are
// all ignored by the filters too: they are still uncommitted.
func (m *Model) hashColor(i int, c git.Commit) color.Color {
	switch {
	case i == 0 && len(m.current.files) > 0:
		return m.pal.RemovedFg
	case i == 0:
		return m.pal.Muted
	case m.hist.unpushed[c.Hash]:
		return m.pal.RemovedFg
	}
	return m.pal.Syntax[syntax.Type]
}

// entrySet returns the files of entry i, or nil while it is not loaded.
func (m *Model) entrySet(i int) *changeSet {
	if i == 0 {
		return m.current
	}
	return m.hist.cache[m.hist.commits[i-1].Hash]
}

// entryCounts draws an entry's number of files and its removed, added,
// modified, changed functions and changed packages totals, colored like
// the file table, or dots while they are not known yet.
func (m *Model) entryCounts(s *changeSet, bg color.Color) string {
	var total diff.Counts

	// Not loaded yet, or computed with other filters.
	if s == nil || s.filters != m.filters {
		return m.pal.Style(m.pal.Muted, bg, false).Render(padLeft("·", filesColWidth) + strings.Repeat(padLeft("·", countColWidth), 5))
	}

	total = s.totals()
	return m.pal.Style(m.pal.Muted, bg, false).Render(padLeft(fileCount(len(s.list.meaningful)), filesColWidth)) +
		m.pal.Style(m.pal.RemovedFg, bg, false).Render(padLeft(fmt.Sprint(total.Removed), countColWidth)) +
		m.pal.Style(m.pal.AddedFg, bg, false).Render(padLeft(fmt.Sprint(total.Added), countColWidth)) +
		m.pal.Style(m.pal.ModifiedFg, bg, false).Render(padLeft(fmt.Sprint(total.Modified), countColWidth)) +
		m.pal.Style(m.pal.Fg, bg, false).Render(padLeft(fmt.Sprint(s.funcTotal()), countColWidth)) +
		m.pal.Style(m.pal.Fg, bg, false).Render(padLeft(fmt.Sprint(s.packageTotal()), countColWidth))
}

// currentSubject describes the "current" entry.
func (m *Model) currentSubject() string {
	if len(m.current.list.meaningful) == 0 {
		return m.current.rangeName + "  (no changes)"
	}
	return m.current.rangeName
}

// branchText describes the branch for the history title, like
// "  main → origin/main ↑2 ↓1", or "" when it is not known.
func (m *Model) branchText() string {
	var st = m.status
	var text string

	if st.Branch == "" {
		return ""
	}

	text = "  " + st.Branch
	if st.Upstream == "" {
		return text + " (no upstream)"
	}
	text += " → " + st.Upstream
	if st.Ahead > 0 {
		text += fmt.Sprintf(" ↑%d", st.Ahead)
	}
	if st.Behind > 0 {
		text += fmt.Sprintf(" ↓%d", st.Behind)
	}
	return text
}

// titledRule draws "── Title ─────" across width columns.
func (m *Model) titledRule(title string, width int) string {
	var head string

	head = "── " + title + " "
	return m.pal.Paint(head, m.pal.Muted) + m.pal.Paint(strings.Repeat("─", max(width-len([]rune(head)), 0)), m.pal.Border)
}

// fileCount formats "1 file" or "12 files".
func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}
