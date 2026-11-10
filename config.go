package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/flo8/go-diff/internal/semantic"
	"github.com/flo8/go-diff/internal/theme"
)

// config is the optional file $XDG_CONFIG_HOME/godiff/config.json
// (~/.config/godiff/config.json on Linux). Every field is optional:
//
//	{
//	  "theme": "light",
//	  "filters": {"comments": true, "whitespace": true, "formatting": true, "imports": false},
//	  "themes": [
//	    {"name": "mine", "base": "dark", "added_bg": "#123b22", "syntax": {"keyword": "#ff79c6"}}
//	  ]
//	}
type config struct {
	Theme   string            `json:"theme"`
	Filters semantic.Filters  `json:"filters"`
	Themes  []json.RawMessage `json:"themes"`
}

// configPath returns where the config file lives.
func configPath() (string, error) {
	var dir string
	var err error

	dir, err = os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "godiff", "config.json"), nil
}

// loadConfig reads the config file at path. A missing file gives the
// defaults. Filters not named in the file keep their default value.
func loadConfig(path string) (config, error) {
	var cfg config
	var data []byte
	var err error

	cfg.Filters = semantic.DefaultFilters

	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// allThemes returns the built-in themes followed by the custom ones.
// A custom theme with a built-in name replaces it.
func allThemes(cfg config) ([]theme.Theme, error) {
	var themes []theme.Theme
	var custom theme.Theme
	var err error

	themes = theme.Builtins()

	for _, raw := range cfg.Themes {
		custom, err = theme.ParseCustom(raw)
		if err != nil {
			return nil, err
		}
		themes = replaceOrAppend(themes, custom)
	}

	return themes, nil
}

// replaceOrAppend puts t in the list, replacing a theme of the same name.
func replaceOrAppend(themes []theme.Theme, t theme.Theme) []theme.Theme {
	for i := range themes {
		if themes[i].Name == t.Name {
			themes[i] = t
			return themes
		}
	}
	return append(themes, t)
}
