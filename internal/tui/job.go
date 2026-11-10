package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/git"
)

const (
	spinDelay   = 80 * time.Millisecond // time between spinner frames
	resultDelay = 4 * time.Second       // a success stays this long on screen
	barWidth    = 16
)

// spinFrames are drawn one after the other while git runs.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// progressLine matches git's progress output, with or without the
// "remote: " prefix: "Writing objects:  45% (9/20)".
var progressLine = regexp.MustCompile(`^(?:remote: )?([A-Za-z][A-Za-z ]*):\s+(\d+)%`)

// jobKind is the git command a job runs.
type jobKind int

const (
	jobCommit jobKind = iota
	jobPull
	jobPush
)

// job is a commit, pull or push running in the background. Only one
// runs at a time.
type job struct {
	id      int
	kind    jobKind
	verb    string // "pushing to origin/main"
	start   time.Time
	frame   int     // spinner frame
	phase   string  // "Writing objects", from git's progress
	percent int     // of the phase, -1 when git did not say
	wait    tea.Cmd // waits for the next jobLine or jobDone
}

// result is the outcome of the last action, shown in the footer: a
// success for a few seconds, an error until the next key.
type result struct {
	id     int
	text   string // first line
	detail []string
	err    bool
}

// jobLine is one line git printed; jobDone ends the job.
type jobLine struct {
	id   int
	line string
}

type jobDone struct {
	id      int
	summary string
	err     error
}

// spinTick redraws the spinner; clearResult hides a success.
type spinTick struct {
	id int
}

type clearResult struct {
	id int
}

func (t spinTick) msg(time.Time) tea.Msg {
	return t
}

func (c clearResult) msg(time.Time) tea.Msg {
	return c
}

// jobRun runs the git command of a job in its own goroutine. Each output
// line goes to lines; when git exits, lines is closed and the outcome is
// sent to done.
type jobRun struct {
	id      int
	kind    jobKind
	repo    *git.Repo
	status  git.WorkStatus // before the command
	message string         // commit message
	lines   chan string
	done    chan jobDone
}

func (j jobRun) run() {
	var out string
	var err error

	switch j.kind {
	case jobCommit:
		out, err = j.repo.Commit(j.message, j.lines)
	case jobPull:
		out, err = j.repo.Pull(j.status, j.lines)
	case jobPush:
		out, err = j.repo.Push(j.status, j.lines)
	}

	close(j.lines)
	j.done <- jobDone{id: j.id, summary: j.summary(out), err: err}
}

// summary says what a job did, for the result line.
func (j jobRun) summary(out string) string {
	switch j.kind {
	case jobPull:
		if strings.Contains(out, "Already up to date") {
			return "already up to date with " + j.status.Upstream
		}
		return "pulled from " + j.status.Upstream
	case jobPush:
		if j.status.Upstream == "" {
			return "pushed " + j.status.Branch + ", upstream set"
		}
		return fmt.Sprintf("pushed %s to %s", commitCount(j.status.Ahead), j.status.Upstream)
	}
	return out
}

// next is a Bubble Tea command that waits for the job's next message:
// an output line, or its outcome once the lines are done.
func (j jobRun) next() tea.Msg {
	line, ok := <-j.lines
	if ok {
		return jobLine{id: j.id, line: line}
	}
	return <-j.done
}

// startJob starts a git command in the background, with the spinner.
func (m *Model) startJob(kind jobKind, verb, message string) tea.Cmd {
	var run jobRun

	m.jobSeq++
	m.job = &job{id: m.jobSeq, kind: kind, verb: verb, start: time.Now(), percent: -1}
	m.result = nil

	run = jobRun{
		id: m.jobSeq, kind: kind, repo: m.repo, status: m.status, message: message,
		lines: make(chan string, 16), done: make(chan jobDone, 1),
	}
	m.job.wait = run.next
	go run.run()

	return tea.Batch(run.next, m.spin())
}

// spin schedules the next spinner frame.
func (m *Model) spin() tea.Cmd {
	return tea.Tick(spinDelay, spinTick{id: m.job.id}.msg)
}

// jobProgress records a line of git's output and waits for the next.
func (m *Model) jobProgress(msg jobLine) tea.Cmd {
	var parts []string

	if m.job == nil || m.job.id != msg.id {
		return nil
	}

	parts = progressLine.FindStringSubmatch(msg.line)
	if parts != nil {
		m.job.phase = strings.TrimSpace(parts[1])
		m.job.percent, _ = strconv.Atoi(parts[2])
	}
	return m.job.wait
}

// spinNext moves the spinner one frame while the job runs.
func (m *Model) spinNext(t spinTick) tea.Cmd {
	if m.job == nil || m.job.id != t.id {
		return nil
	}
	m.job.frame++
	return m.spin()
}

// hideResult clears a success once its time is up.
func (m *Model) hideResult(c clearResult) {
	if m.result != nil && m.result.id == c.id && !m.result.err {
		m.result = nil
	}
}

// finishJob shows the outcome and, on success, reloads what the command
// changed.
func (m *Model) finishJob(msg jobDone) tea.Cmd {
	var elapsed time.Duration

	if m.job == nil || m.job.id != msg.id {
		return nil
	}
	elapsed = time.Since(m.job.start)
	m.job = nil

	return m.showResult(msg.summary, msg.err, elapsed)
}

// showResult puts an outcome in the footer and reloads the repository
// state, which may have changed even when the command failed.
func (m *Model) showResult(text string, err error, elapsed time.Duration) tea.Cmd {
	var lines []string
	var head int

	m.jobSeq++
	m.result = &result{id: m.jobSeq, text: fmt.Sprintf("✓ %s  %.1fs", text, elapsed.Seconds())}

	// With no delay (in tests) a success stays until the next key.
	if err == nil && m.resultDelay == 0 {
		return m.refresh()
	}
	if err == nil {
		return tea.Batch(m.refresh(), tea.Tick(m.resultDelay, clearResult{id: m.jobSeq}.msg))
	}

	lines = strings.Split(err.Error(), "\n")
	head = headline(lines)
	m.result.text = "✗ " + lines[head]
	m.result.detail = append(lines[:head:head], lines[head+1:]...)
	m.result.err = true
	return m.refresh()
}

// headline picks the line of git's output that says what went wrong:
// a rejected ref, then a fatal error, then an error, else the first line.
func headline(lines []string) int {
	for _, prefix := range []string{"! [", "fatal:", "error:"} {
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				return i
			}
		}
	}
	return 0
}

// showError puts an error in the footer without running anything.
func (m *Model) showError(text string) {
	m.jobSeq++
	m.result = &result{id: m.jobSeq, text: "✗ " + text, err: true}
}

// jobStatusLine draws the running job: spinner, what it does, a progress bar
// when git reports one, and the elapsed time.
func (m *Model) jobStatusLine() string {
	var j *job
	var b strings.Builder

	j = m.job
	b.WriteString(m.pal.Style(m.pal.AddedFg, m.pal.Bg, true).Render(" " + spinFrames[j.frame%len(spinFrames)] + " "))
	b.WriteString(m.pal.Paint(j.verb, m.pal.Fg))

	if j.percent >= 0 {
		b.WriteString("  " + m.progressBar(j.percent) + "  ")
		b.WriteString(m.pal.Paint(fmt.Sprintf("%s %d%%", j.phase, j.percent), m.pal.Muted))
	}

	b.WriteString(m.pal.Paint(fmt.Sprintf("   %.1fs", time.Since(j.start).Seconds()), m.pal.Muted))
	return m.fit(b.String())
}

// progressBar draws percent as a bar of barWidth cells.
func (m *Model) progressBar(percent int) string {
	var filled int

	filled = min(max(percent, 0), 100) * barWidth / 100
	return m.pal.Paint(strings.Repeat("█", filled), m.pal.AddedFg) +
		m.pal.Paint(strings.Repeat("░", barWidth-filled), m.pal.Border)
}

// resultLine draws the last outcome, mint for a success, pink for an
// error.
func (m *Model) resultLine() string {
	if m.result.err {
		return m.fit(m.pal.Style(m.pal.RemovedFg, m.pal.Bg, true).Render(" " + m.result.text))
	}
	return m.fit(m.pal.Style(m.pal.AddedFg, m.pal.Bg, true).Render(" " + m.result.text))
}

// resultDetail draws the rest of an error's output above the footer.
func (m *Model) resultDetail() []string {
	var lines []string

	if m.result == nil || !m.result.err {
		return nil
	}
	for _, line := range m.result.detail {
		lines = append(lines, m.fit(m.pal.Paint("   "+line, m.pal.RemovedFg)))
	}
	return lines
}

// commitCount formats "1 commit" or "3 commits".
func commitCount(n int) string {
	if n == 1 {
		return "1 commit"
	}
	return fmt.Sprintf("%d commits", n)
}
