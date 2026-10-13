package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
	"github.com/arnaudmorin/gorrit/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if errors.Is(err, config.ErrNotFound) {
		cfg, err = setup()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := gerrit.New(cfg.Gerrit.URL, cfg.Gerrit.Username, cfg.Gerrit.Token)
	p := tea.NewProgram(ui.New(client, ui.Options{DiffCommand: cfg.Diff.CommandOrDefault(), Filters: cfg.FiltersOrDefault(), BugURL: cfg.Bugs.URLOrDefault(), Bots: cfg.Messages.Bots}), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
