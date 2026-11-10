// Command godiff reviews Git changes in the terminal, showing only the
// changes that matter: whitespace, formatting and comment edits are
// filtered out, and changed words are highlighted inside changed lines.
//
// Usage:
//
//	godiff                  working tree vs HEAD
//	godiff --staged         staged changes (index vs HEAD)
//	godiff --unstaged       unstaged changes (working tree vs index)
//	godiff --commit REV     one commit vs its parent
//	godiff BASE             working tree vs BASE
//	godiff BASE TARGET      BASE vs TARGET (commits or branches)
//	godiff A..B             same as godiff A B
//	godiff A...B            B vs the point where it forked from A
//
// godiff only reads the repository. It never changes files or the index.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/flo8/go-diff/internal/git"
	"github.com/flo8/go-diff/internal/review"
	"github.com/flo8/go-diff/internal/theme"
	"github.com/flo8/go-diff/internal/tui"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "godiff:", err)
		os.Exit(1)
	}
}

// run parses the command line, loads the changes and starts the interface.
func run() error {
	var opt git.Options
	var themeName string
	var cfgPath string
	var cfg config
	var themes []theme.Theme
	var repo *git.Repo
	var rg git.Range
	var files []*review.File
	var err error

	defaultCfg, _ := configPath()
	flag.BoolVar(&opt.Staged, "staged", false, "show staged changes (index vs HEAD)")
	flag.BoolVar(&opt.Unstaged, "unstaged", false, "show unstaged changes (working tree vs index)")
	flag.StringVar(&opt.Commit, "commit", "", "show one commit against its parent")
	flag.StringVar(&themeName, "theme", "", "theme name: dark, light, high-contrast, ansi or a custom one")
	flag.StringVar(&cfgPath, "config", defaultCfg, "config file")
	flag.Usage = usage
	flag.Parse()
	opt.Args = flag.Args()

	// Configuration: filters and themes.
	cfg, err = loadConfig(cfgPath)
	if err != nil {
		return err
	}
	themes, err = allThemes(cfg)
	if err != nil {
		return err
	}
	if themeName == "" {
		themeName = cfg.Theme
	}
	if themeName != "" {
		_, err = theme.Find(themes, themeName)
		if err != nil {
			return err
		}
	}

	// Repository, range and file contents.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err = git.Open(cwd)
	if err != nil {
		return err
	}
	rg, err = repo.ResolveRange(opt)
	if err != nil {
		return err
	}
	files, err = review.Load(repo, rg)
	if err != nil {
		return err
	}
	review.ComputeAll(files, cfg.Filters)

	model := tui.New(tui.Options{
		Repo:    repo,
		Files:   files,
		Range:   rg.String(),
		Filters: cfg.Filters,
		Themes:  themes,
		Theme:   themeName,
	})

	_, err = tea.NewProgram(model).Run()
	return err
}

// usage prints the command-line help.
func usage() {
	fmt.Fprint(os.Stderr, `Usage: godiff [flags] [BASE [TARGET] | A..B | A...B]

Review the meaningful changes between two versions of a Git repository.

  godiff                  working tree vs HEAD
  godiff --staged         staged changes (index vs HEAD)
  godiff --unstaged       unstaged changes (working tree vs index)
  godiff --commit REV     one commit vs its parent
  godiff BASE             working tree vs BASE
  godiff BASE TARGET      BASE vs TARGET (commits or branches)
  godiff A...B            B vs the point where it forked from A

Flags:
`)
	flag.PrintDefaults()
}
