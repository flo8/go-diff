package theme

import (
	"encoding/json"
	"testing"
)

// Scenario: a custom theme based on "light" that overrides two colors,
// one of them inside the nested syntax colors.
// Goal: overridden fields change, every other field comes from the base,
// and the built-in theme itself is untouched.
func TestParseCustomOverridesBase(t *testing.T) {
	raw := json.RawMessage(`{"name": "mine", "base": "light", "added_bg": "#00ff00", "syntax": {"keyword": "#ff00ff"}}`)

	got, err := ParseCustom(raw)
	if err != nil {
		t.Fatal(err)
	}

	base := light()
	if got.Name != "mine" || got.AddedBg != "#00ff00" || got.Syntax.Keyword != "#ff00ff" {
		t.Errorf("overrides not applied: %+v", got)
	}
	if got.RemovedBg != base.RemovedBg || got.Syntax.String != base.Syntax.String || got.Dark {
		t.Errorf("fields not inherited from base: %+v", got)
	}

	again, _ := Find(Builtins(), "light")
	if again.AddedBg != base.AddedBg {
		t.Errorf("built-in theme was modified")
	}
}

// Scenario: invalid custom themes.
// Goal: a clear error instead of a silently broken theme.
func TestParseCustomErrors(t *testing.T) {
	cases := []string{
		`{"base": "dark"}`,              // no name
		`{"name": "x", "base": "nope"}`, // unknown base
		`{"name": "x", "added_bg": 12}`, // wrong type
	}

	for _, c := range cases {
		_, err := ParseCustom(json.RawMessage(c))
		if err == nil {
			t.Errorf("%s: expected an error", c)
		}
	}
}
