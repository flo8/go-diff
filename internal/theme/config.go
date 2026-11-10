package theme

import (
	"encoding/json"
	"fmt"
)

// ParseCustom builds a theme from a JSON object. The object names a
// built-in theme in "base" (default "dark") and overrides any of its
// fields, for example:
//
//	{"name": "mine", "base": "light", "added_bg": "#d0f0d0",
//	 "syntax": {"keyword": "#aa00aa"}}
func ParseCustom(raw json.RawMessage) (Theme, error) {
	var header struct {
		Name string `json:"name"`
		Base string `json:"base"`
	}
	var t Theme
	var err error

	err = json.Unmarshal(raw, &header)
	if err != nil {
		return Theme{}, fmt.Errorf("theme: %w", err)
	}
	if header.Name == "" {
		return Theme{}, fmt.Errorf("theme: missing \"name\"")
	}
	if header.Base == "" {
		header.Base = "dark"
	}

	// Start from a copy of the base, then let the JSON overwrite the
	// fields it sets. Theme has no maps or pointers, so the copy is deep.
	t, err = Find(Builtins(), header.Base)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q: %w", header.Name, err)
	}

	err = json.Unmarshal(raw, &t)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q: %w", header.Name, err)
	}

	return t, nil
}

// Find returns the theme with the given name.
func Find(themes []Theme, name string) (Theme, error) {
	for _, t := range themes {
		if t.Name == name {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("unknown theme %q", name)
}
