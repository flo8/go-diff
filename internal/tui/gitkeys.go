package tui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/semantic"
)

// This file holds the keys that change the repository: space and a
// stage, c and C commit, p pulls and P pushes. The git commands
// themselves are in internal/git/actions.go.

// gitKey runs c, C, p and P. It returns false for other keys.
func (m *Model) gitKey(key string) (tea.Cmd, bool) {
	switch key {
	case "c", "C", "p", "P":
	default:
		return nil, false
	}

	if m.repo == nil {
		return nil, true
	}
	if m.job != nil {
		m.showError("wait: " + m.job.verb)
		return nil, true
	}

	switch key {
	case "c":
		m.openCommitInput()
	case "C":
		return m.commitWithEditor(), true
	case "p":
		return m.startJob(jobPull, "pulling from "+m.status.Upstream, ""), true
	case "P":
		return m.startJob(jobPush, m.pushVerb(), ""), true
	}
	return nil, true
}

// pushVerb says where a push goes.
func (m *Model) pushVerb() string {
	var remote string

	if m.status.Upstream != "" {
		return "pushing to " + m.status.Upstream
	}
	remote, _ = m.repo.PushRemote()
	return "pushing " + m.status.Branch + " to " + remote
}

// openCommitInput asks for a one-line commit message, if anything is
// staged.
func (m *Model) openCommitInput() {
	if !m.status.HasStaged() {
		m.showError(m.nothingStaged())
		return
	}
	m.input = &textInput{prompt: "commit message"}
}

// nothingStaged explains why there is nothing to commit.
func (m *Model) nothingStaged() string {
	if m.canStage {
		return "nothing staged: space stages the selected file, a stages all"
	}
	return "nothing staged"
}

// inputKey sends a key to the commit message field. Enter commits, Esc
// cancels, an empty message does nothing.
func (m *Model) inputKey(key, text string) tea.Cmd {
	var message string

	if key == "esc" {
		m.input = nil
		return nil
	}
	if !m.input.edit(key, text) {
		return nil
	}

	message = m.input.value()
	if message == "" {
		return nil
	}
	m.input = nil
	return m.startJob(jobCommit, "committing", message)
}

// editorCommit runs "git commit" in the user's editor. Its error output
// (a failing hook) is copied so it can be shown afterwards.
type editorCommit struct {
	start  time.Time
	stderr *bytes.Buffer
}

// editorDone is sent when the editor commit returns.
type editorDone struct {
	run editorCommit
	err error
}

func (e editorCommit) done(err error) tea.Msg {
	return editorDone{run: e, err: err}
}

// commitWithEditor hands the terminal to "git commit", which opens the
// editor for a full message.
func (m *Model) commitWithEditor() tea.Cmd {
	var run editorCommit
	var cmd = m.repo.CommitCmd()

	if !m.status.HasStaged() {
		m.showError(m.nothingStaged())
		return nil
	}

	run = editorCommit{start: time.Now(), stderr: &bytes.Buffer{}}
	cmd.Stderr = io.MultiWriter(os.Stderr, run.stderr)
	return tea.ExecProcess(cmd, run.done)
}

// finishEditorCommit shows how the editor commit went.
func (m *Model) finishEditorCommit(msg editorDone) tea.Cmd {
	var elapsed time.Duration
	var out string

	elapsed = time.Since(msg.run.start)
	if msg.err != nil {
		out = msg.run.stderr.String()
		if out == "" {
			out = msg.err.Error()
		}
		return m.showResult("", errors.New(strings.TrimSpace(out)), elapsed)
	}
	return m.showResult("committed", nil, elapsed)
}

// stageKey runs space (stage or unstage the selected file) and a (stage
// or unstage everything) on the "current" file table.
func (m *Model) stageKey(key string) {
	var err error

	if key == "a" {
		err = m.toggleStageAll()
	} else {
		err = m.toggleStageSelected()
	}
	if err != nil {
		m.showError(err.Error())
	}

	m.reloadStatus()
}

// toggleStageAll stages everything, or unstages everything when nothing
// is left to stage.
func (m *Model) toggleStageAll() error {
	if m.status.HasUnstaged() {
		return m.repo.StageAll()
	}
	return m.repo.UnstageAll()
}

// toggleStageSelected unstages the selected file when it is fully
// staged, and stages it otherwise. A rename has two paths; staging uses
// the ones git still lists, unstaging both.
func (m *Model) toggleStageSelected() error {
	var l *fileList
	var c git.Change
	var paths []string

	l = &m.current.list
	if len(l.visible) == 0 {
		return nil
	}
	c = l.visible[l.selected].Change

	if m.status.Files[c.Path()] == git.Staged {
		return m.repo.Unstage(changePaths(c))
	}

	for _, path := range changePaths(c) {
		if _, changed := m.status.Files[path]; changed {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return m.repo.Stage(paths)
}

// keepSelection selects in the new list the file that was selected in
// the old one, if it is still there, and keeps the scroll position.
func keepSelection(old, new *fileList) {
	var path string

	if old.selected >= len(old.visible) {
		return
	}
	path = old.visible[old.selected].Change.Path()

	for i, f := range new.visible {
		if f.Change.Path() == path {
			new.selected = i
		}
	}
	new.top = old.top
}

// changePaths returns the old and new paths of a change, without
// duplicates or empty ones.
func changePaths(c git.Change) []string {
	var paths []string

	if c.OldPath != "" {
		paths = append(paths, c.OldPath)
	}
	if c.NewPath != "" && c.NewPath != c.OldPath {
		paths = append(paths, c.NewPath)
	}
	return paths
}

// reloadStatus reads the branch and staging state again.
func (m *Model) reloadStatus() {
	var st git.WorkStatus
	var err error

	st, err = m.repo.Status()
	if err != nil {
		m.showError(err.Error())
		return
	}
	m.status = st
}

// refreshed carries the repository state read again after a commit,
// pull or push.
type refreshed struct {
	set      *changeSet
	commits  []git.Commit
	unpushed map[string]bool
	status   git.WorkStatus
	err      error
}

// refresh returns a Bubble Tea command that reads "current", the
// history and the status again, off the main loop.
func (m *Model) refresh() tea.Cmd {
	m.refreshing = true
	return refreshJob{repo: m.repo, opts: m.rangeOpts, filters: m.filters}.run
}

// autoRefreshTick fires every refreshEvery to pick up changes made
// outside godiff: edited files, commits from the shell, a fetch.
type autoRefreshTick struct{}

func (autoRefreshTick) msg(time.Time) tea.Msg {
	return autoRefreshTick{}
}

// nextAutoRefresh schedules the next automatic refresh, if any.
func (m *Model) nextAutoRefresh() tea.Cmd {
	if m.repo == nil || m.refreshEvery == 0 {
		return nil
	}
	return tea.Tick(m.refreshEvery, autoRefreshTick{}.msg)
}

// autoRefresh reloads the repository state unless it would get in the
// way: while a file is open (its lines would change under the reader),
// a commit message is typed, or a git command or refresh runs.
func (m *Model) autoRefresh() tea.Cmd {
	var busy bool

	busy = m.files.viewer != nil || m.input != nil || m.job != nil || m.refreshing
	if busy || m.repo == nil {
		return m.nextAutoRefresh()
	}
	return tea.Batch(m.refresh(), m.nextAutoRefresh())
}

// refreshJob holds what refresh needs, copied so that the background
// command does not read the model.
type refreshJob struct {
	repo    *git.Repo
	opts    git.Options
	filters semantic.Filters
}

func (j refreshJob) run() tea.Msg {
	var rg git.Range
	var files []*review.File
	var msg refreshed

	rg, msg.err = j.repo.ResolveRange(j.opts)
	if msg.err != nil {
		return msg
	}
	files, msg.err = review.Load(j.repo, rg)
	if msg.err != nil {
		return msg
	}
	review.ComputeAll(files, j.filters)
	msg.set = newChangeSet(files, rg.String(), j.filters)

	msg.commits, msg.err = j.repo.Log(logLimit)
	if msg.err != nil {
		return msg
	}
	msg.status, msg.err = j.repo.Status()
	if msg.err != nil {
		return msg
	}
	msg.unpushed, msg.err = j.repo.Unpushed(msg.status.Upstream)
	return msg
}

// applyRefresh shows the state read again. The history keeps the same
// commit selected; loaded commits stay cached since a commit never
// changes.
func (m *Model) applyRefresh(msg refreshed) tea.Cmd {
	var commit git.Commit
	var isCommit bool

	m.refreshing = false
	if msg.err != nil {
		m.showError(msg.err.Error())
		return nil
	}

	commit, isCommit = m.hist.selectedCommit()
	m.applySort(msg.set)
	keepSelection(&m.current.list, &msg.set.list)
	m.current = msg.set
	m.hist.commits = msg.commits
	m.hist.unpushed = msg.unpushed
	m.status = msg.status

	m.hist.selected = 0
	for i, c := range m.hist.commits {
		if isCommit && c.Hash == commit.Hash {
			m.hist.selected = i + 1
		}
	}

	// Shows the entry again; this closes an open viewer, whose file
	// may be gone.
	return m.selectEntry(m.hist.selected)
}
