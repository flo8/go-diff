package tui

import (
	"strings"
)

// textInput is a one-line text field: typed text is added at the end,
// there is no cursor to move.
type textInput struct {
	prompt string
	text   []rune
}

// edit applies one key. It returns true when the key was Enter.
// key is the key's name ("enter", "backspace"), text what it types.
func (in *textInput) edit(key, text string) bool {
	switch key {
	case "enter":
		return true
	case "backspace":
		in.text = in.text[:max(len(in.text)-1, 0)]
	case "ctrl+u":
		in.text = nil
	case "ctrl+w":
		in.deleteWord()
	default:
		in.insert(text)
	}
	return false
}

// insert adds typed or pasted text; a pasted newline becomes a space
// since the message is one line.
func (in *textInput) insert(text string) {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	in.text = append(in.text, []rune(text)...)
}

// deleteWord removes the last word and the spaces after it.
func (in *textInput) deleteWord() {
	var n int

	n = len(in.text)
	for n > 0 && in.text[n-1] == ' ' {
		n--
	}
	for n > 0 && in.text[n-1] != ' ' {
		n--
	}
	in.text = in.text[:n]
}

// value returns the text without surrounding spaces.
func (in *textInput) value() string {
	return strings.TrimSpace(string(in.text))
}

// inputLine draws the field in the footer, with a block as the cursor.
// A text too long for the line shows its end, where the typing happens.
func (m *Model) inputLine() string {
	var room int
	var text []rune
	var line string

	room = max(m.width-len(m.input.prompt)-30, 10)
	text = m.input.text
	if len(text) > room {
		text = append([]rune("…"), text[len(text)-room+1:]...)
	}

	line = m.pal.Style(m.pal.Header, m.pal.Bg, true).Render(" "+m.input.prompt+": ") +
		m.pal.Paint(string(text), m.pal.Fg) +
		m.pal.Paint("█", m.pal.AddedFg) +
		m.pal.Paint("   ⏎ commit  esc cancel", m.pal.Muted)

	return m.fit(line)
}
