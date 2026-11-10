package diff

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// sideOf builds a Side where every line is its own key and nothing is ignored,
// except lines starting with "#", which play the role of comments.
func sideOf(text string) Side {
	var s Side

	if text == "" {
		return s
	}
	for _, line := range strings.Split(text, "\n") {
		s.Keys = append(s.Keys, line)
		s.Ignored = append(s.Ignored, strings.HasPrefix(line, "#"))
	}
	return s
}

// Scenario: classic LCS cases, including empty inputs.
// Goal: Match returns the expected common pairs in order.
func TestMatch(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want []Pair
	}{
		{"both empty", nil, nil, nil},
		{"old empty", nil, []string{"x"}, nil},
		{"identical", []string{"a", "b"}, []string{"a", "b"}, []Pair{{0, 0}, {1, 1}}},
		{"insert in middle", []string{"a", "c"}, []string{"a", "b", "c"}, []Pair{{0, 0}, {1, 2}}},
		{"delete in middle", []string{"a", "b", "c"}, []string{"a", "c"}, []Pair{{0, 0}, {2, 1}}},
		{"all different", []string{"a"}, []string{"b"}, nil},
		// Two answers have the same length; Myers takes deletions first.
		{"swap", []string{"a", "b", "c", "d"}, []string{"b", "a", "d", "c"}, []Pair{{1, 0}, {3, 2}}},
	}

	for _, c := range cases {
		got := Match(c.a, c.b)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}
}

// Scenario: a 60,000-line file where every 20th line was edited, which is
// past the Myers edit-distance limit.
// Goal: the unchanged lines are still matched (anchor fallback), the
// edited lines count as modified, and it stays fast.
func TestAlignLargeFileManyEdits(t *testing.T) {
	var old, new Side
	const n = 60000

	for i := 0; i < n; i++ {
		line := fmt.Sprintf("line %d", i)
		old.Keys = append(old.Keys, line)
		if i%20 == 0 {
			line += " edited"
		}
		new.Keys = append(new.Keys, line)
	}
	old.Ignored = make([]bool, n)
	new.Ignored = make([]bool, n)

	got := Count(Align(old, new))
	want := Counts{Modified: n / 20}
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
}

// Scenario: two edits in one file, one balanced and one unbalanced.
// Goal: a removed line next to an added line counts as one modification,
// and the extra added line counts as an addition.
func TestAlignCountsModifiedPairs(t *testing.T) {
	old := sideOf("a\nb\nc\nd")
	new := sideOf("a\nB\nc\ne\nf")

	got := Count(Align(old, new))
	want := Counts{Removed: 0, Added: 1, Modified: 2}
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
}

// Scenario: a block is deleted, another one is inserted elsewhere.
// Goal: pure deletions and pure insertions are not paired across matches.
func TestAlignSeparateRemovalAndAddition(t *testing.T) {
	old := sideOf("a\nx\ny\nb\nc")
	new := sideOf("a\nb\nc\nz")

	got := Count(Align(old, new))
	want := Counts{Removed: 2, Added: 1, Modified: 0}
	if got != want {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
}

// Scenario: only "comment" lines (ignored) were added, removed and changed.
// Goal: no change is counted, and every line still appears in a row.
func TestAlignIgnoredLinesAreNotChanges(t *testing.T) {
	old := sideOf("a\n# old note\nb")
	new := sideOf("# header\na\nb\n# new note")

	rows := Align(old, new)
	got := Count(rows)
	if got.Total() != 0 {
		t.Errorf("counts = %+v, want none", got)
	}

	// Every old and new line must still be shown somewhere.
	seenOld := map[int]bool{}
	seenNew := map[int]bool{}
	for _, r := range rows {
		seenOld[r.Old] = true
		seenNew[r.New] = true
	}
	for i := 0; i < 3; i++ {
		if !seenOld[i] {
			t.Errorf("old line %d missing from rows", i)
		}
	}
	for i := 0; i < 4; i++ {
		if !seenNew[i] {
			t.Errorf("new line %d missing from rows", i)
		}
	}
}

// Scenario: file created and file deleted.
// Goal: every line counts as added (or removed), never modified.
func TestAlignNewAndDeletedFile(t *testing.T) {
	created := Count(Align(Side{}, sideOf("a\nb\n# c")))
	if created != (Counts{Added: 2}) {
		t.Errorf("new file counts = %+v, want 2 added", created)
	}

	deleted := Count(Align(sideOf("a\nb\nc"), Side{}))
	if deleted != (Counts{Removed: 3}) {
		t.Errorf("deleted file counts = %+v, want 3 removed", deleted)
	}
}

// texts returns the highlighted substrings.
func texts(s string, spans []Span) []string {
	var out []string
	for _, sp := range spans {
		out = append(out, s[sp.Start:sp.End])
	}
	return out
}

// Scenario: the example from the brief, a number changes in a statement.
// Goal: only "500" and "1000" are highlighted.
func TestWordsHighlightsOnlyChangedNumber(t *testing.T) {
	a := "timeout := 500 * time.Millisecond"
	b := "timeout := 1000 * time.Millisecond"

	sa, sb := Words(a, b, false)
	if got := texts(a, sa); !reflect.DeepEqual(got, []string{"500"}) {
		t.Errorf("old spans = %q, want [500]", got)
	}
	if got := texts(b, sb); !reflect.DeepEqual(got, []string{"1000"}) {
		t.Errorf("new spans = %q, want [1000]", got)
	}
}

// Scenario: two adjacent words change.
// Goal: they are merged into one span across the space between them.
func TestWordsMergesAcrossWhitespace(t *testing.T) {
	a := "return foo bar"
	b := "return baz qux"

	sa, sb := Words(a, b, false)
	if got := texts(a, sa); !reflect.DeepEqual(got, []string{"foo bar"}) {
		t.Errorf("old spans = %q, want [foo bar]", got)
	}
	if got := texts(b, sb); !reflect.DeepEqual(got, []string{"baz qux"}) {
		t.Errorf("new spans = %q, want [baz qux]", got)
	}
}

// Scenario: an unknown file type uses character-level comparison.
// Goal: only the changed character is highlighted.
func TestWordsCharacterLevel(t *testing.T) {
	a := "colour"
	b := "color"

	sa, sb := Words(a, b, true)
	if got := texts(a, sa); !reflect.DeepEqual(got, []string{"u"}) {
		t.Errorf("old spans = %q, want [u]", got)
	}
	if len(sb) != 0 {
		t.Errorf("new spans = %q, want none", texts(b, sb))
	}
}

// Scenario: indentation is the only difference.
// Goal: whitespace is never highlighted on its own.
func TestWordsIgnoresWhitespaceOnly(t *testing.T) {
	sa, sb := Words("\tx := 1", "    x := 1", false)
	if len(sa) != 0 || len(sb) != 0 {
		t.Errorf("spans = %v %v, want none", sa, sb)
	}
}
