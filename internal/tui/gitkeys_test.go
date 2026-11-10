package tui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/syntax"
)

// enter presses Enter the way the terminal sends it.
func enter(m *Model) {
	send(m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

// Scenario: the test repository has b.go modified and not staged.
// Goal: space stages it (● mark), space again unstages it (○), and a
// stages then unstages everything.
func TestStageKeys(t *testing.T) {
	isolateGit(t)
	m := newTestModel(t, true)

	if m.status.Files["b.go"] != git.Unstaged || !strings.Contains(ansi.Strip(m.mainView()), "○") {
		t.Fatalf("b.go should start unstaged with a ○ mark")
	}

	press(m, "space")
	if m.status.Files["b.go"] != git.Staged || !strings.Contains(ansi.Strip(m.mainView()), "●") {
		t.Errorf("space should stage b.go: %v", m.status.Files)
	}

	press(m, "space")
	if m.status.Files["b.go"] != git.Unstaged {
		t.Errorf("space again should unstage b.go: %v", m.status.Files)
	}

	press(m, "a")
	if m.status.HasUnstaged() {
		t.Errorf("a should stage everything: %v", m.status.Files)
	}
	press(m, "a")
	if m.status.HasStaged() {
		t.Errorf("a again should unstage everything: %v", m.status.Files)
	}
}

// Scenario: press c with nothing staged.
// Goal: no message field opens; the footer explains what to do.
func TestCommitNothingStaged(t *testing.T) {
	m := newTestModel(t, true)

	press(m, "c")
	if m.input != nil {
		t.Errorf("the message field should not open")
	}
	if !strings.Contains(ansi.Strip(m.mainFooter()), "nothing staged") {
		t.Errorf("footer = %q", ansi.Strip(m.mainFooter()))
	}
}

// Scenario: stage b.go, press c, type a message, press Enter.
// Goal: the commit is made; the history shows it first, "current" is
// empty, and the footer reports the commit.
func TestCommitFromInput(t *testing.T) {
	isolateGit(t)
	m := newTestModel(t, true)

	press(m, "space")
	press(m, "c")
	if m.input == nil {
		t.Fatalf("c should open the message field")
	}
	send(m, tea.PasteMsg{Content: "update b"})
	enter(m)

	if m.input != nil || m.job != nil {
		t.Fatalf("the field should close and the job end")
	}
	if len(m.hist.commits) != 4 || m.hist.commits[0].Subject != "update b" {
		t.Errorf("history should start with the new commit: %+v", m.hist.commits[0])
	}
	if len(m.current.list.visible) != 0 {
		t.Errorf("current should be empty after the commit")
	}
	if m.result == nil || m.result.err || !strings.Contains(m.result.text, "update b") {
		t.Errorf("result = %+v", m.result)
	}
}

// Scenario: push a branch without upstream to a local bare repository.
// Goal: the push succeeds, sets origin/main as upstream, and the
// history title shows it.
func TestPushSetsUpstream(t *testing.T) {
	isolateGit(t)
	m := newTestModel(t, true)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, m.repo.Root, "remote", "add", "origin", bare)

	press(m, "P")

	if m.result == nil || m.result.err {
		t.Fatalf("push failed: %+v", m.result)
	}
	if m.status.Upstream != "origin/main" {
		t.Errorf("upstream = %q", m.status.Upstream)
	}
	if !strings.Contains(ansi.Strip(m.mainView()), "main → origin/main") {
		t.Errorf("history title should show the upstream")
	}
	out, _ := exec.Command("git", "-C", bare, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "add b" {
		t.Errorf("remote head = %q", out)
	}
}

// Scenario: the test repository is on main, then on a detached HEAD.
// Goal: the header names the branch, then says "detached".
func TestHeaderBranch(t *testing.T) {
	m := newTestModel(t, true)
	if !strings.Contains(ansi.Strip(m.mainView()), " godiff  main  HEAD → working tree") {
		t.Errorf("header should name the branch: %q", ansi.Strip(m.headerLine("x")))
	}

	runGit(t, m.repo.Root, "checkout", "-q", "--detach")
	m.reloadStatus()
	if !strings.Contains(ansi.Strip(m.headerLine("x")), " detached ") {
		t.Errorf("header should say detached: %q", ansi.Strip(m.headerLine("x")))
	}
}

// Scenario: pull with no upstream.
// Goal: the error shows in the footer and goes away on the next key.
func TestPullErrorClearsOnKey(t *testing.T) {
	m := newTestModel(t, true)

	press(m, "p")
	if m.result == nil || !m.result.err || !strings.Contains(m.result.text, "no upstream") {
		t.Fatalf("result = %+v", m.result)
	}

	press(m, "j")
	if m.result != nil {
		t.Errorf("a key should clear the error")
	}
}

// Scenario: git reports "Writing objects:  45% (9/20)" while a push runs.
// Goal: the footer shows the spinner, the phase, the percentage and a
// bar 7 cells out of 16 full.
func TestJobProgressLine(t *testing.T) {
	m := newTestModel(t, true)
	m.job = &job{id: 1, verb: "pushing to origin/main", start: time.Now(), percent: -1}
	m.jobSeq = 1

	m.jobProgress(jobLine{id: 1, line: "remote: Writing objects:  45% (9/20)"})
	if m.job.phase != "Writing objects" || m.job.percent != 45 {
		t.Fatalf("phase %q percent %d", m.job.phase, m.job.percent)
	}

	line := ansi.Strip(m.mainFooter())
	for _, want := range []string{"⠋", "pushing to origin/main", "███████░░░░░░░░░", "Writing objects 45%"} {
		if !strings.Contains(line, want) {
			t.Errorf("footer %q lacks %q", line, want)
		}
	}
}

// Scenario: a push rejected by the remote, as git prints it.
// Goal: the rejected line is the headline; the other lines, in order,
// are the detail.
func TestErrorHeadline(t *testing.T) {
	m := newTestModel(t, true)
	out := "To /tmp/remote.git\n! [rejected]        main -> main (fetch first)\nerror: failed to push some refs"

	m.showResult("", errors.New(out), 0)

	if m.result.text != "✗ ! [rejected]        main -> main (fetch first)" {
		t.Errorf("headline = %q", m.result.text)
	}
	if len(m.result.detail) != 2 || m.result.detail[0] != "To /tmp/remote.git" {
		t.Errorf("detail = %q", m.result.detail)
	}
}

// Scenario: edit a commit message: type, delete a word, paste two lines.
// Goal: the field holds one line with the expected text.
func TestTextInput(t *testing.T) {
	in := &textInput{}

	in.insert("fix the")
	in.edit("ctrl+w", "")
	in.edit("backspace", "")
	in.insert(" bug\nin b")

	if got := string(in.text); got != "fix bug in b" {
		t.Errorf("text = %q", got)
	}
	if !in.edit("enter", "") {
		t.Errorf("enter should submit")
	}
}

// Scenario: the test repository was never pushed, then is pushed.
// Goal: before, every commit hash is pink (unpushed); after, purple.
// "current" is pink while b.go has changes.
func TestUnpushedHashColor(t *testing.T) {
	isolateGit(t)
	m := newTestModel(t, true)
	bare := t.TempDir()
	runGit(t, bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, m.repo.Root, "remote", "add", "origin", bare)

	if m.hashColor(1, m.hist.commits[0]) != m.pal.RemovedFg {
		t.Errorf("a commit that was never pushed should be pink")
	}

	press(m, "P")
	for i, c := range m.hist.commits {
		if m.hashColor(i+1, c) != m.pal.Syntax[syntax.Type] {
			t.Errorf("%s should be purple after the push", c.Short)
		}
	}
	if m.hashColor(0, git.Commit{}) != m.pal.RemovedFg {
		t.Errorf("current with changes should be pink")
	}
}

// Scenario: commit every change, so the working tree is clean.
// Goal: "current" turns from pink to purple.
func TestCurrentColorWhenClean(t *testing.T) {
	isolateGit(t)
	m := newTestModel(t, true)

	press(m, "a")
	press(m, "c")
	send(m, tea.PasteMsg{Content: "update b"})
	enter(m)

	if m.hashColor(0, git.Commit{}) != m.pal.Syntax[syntax.Type] {
		t.Errorf("current without changes should be purple")
	}
}
