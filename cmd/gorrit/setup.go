package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/arnaudmorin/gorrit/internal/config"
)

// setup interactively creates the config file on first run.
func setup() (config.Config, error) {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return config.Config{}, fmt.Errorf("no config file at %s and stdin is not a terminal", config.Path())
	}
	fmt.Printf("No config found, creating %s\n\n", config.Path())
	in := bufio.NewReader(os.Stdin)

	url := prompt(in, fmt.Sprintf("Gerrit URL [%s]: ", config.DefaultURL))
	if url == "" {
		url = config.DefaultURL
	}
	url = strings.TrimRight(url, "/")
	username := prompt(in, "Username (empty for anonymous access): ")
	var token string
	if username != "" {
		fmt.Printf("\nGenerate an HTTP token at %s/settings/#HTTPCredentials\n", url)
		fmt.Print("HTTP token: ")
		raw, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Println()
		if err != nil {
			return config.Config{}, err
		}
		token = strings.TrimSpace(string(raw))
	}

	cfg := config.Config{
		Gerrit:   config.Gerrit{URL: url, Username: username, Token: token},
		Diff:     config.Diff{Command: config.DefaultDiffCommand},
		Bugs:     config.Bugs{URL: config.DefaultBugURL},
		Messages: config.Messages{Bots: []string{"Zuul"}},
		Filters:  config.DefaultFilters,
	}
	if username == "" {
		cfg.Filters = config.AnonymousFilters
	}
	if err := config.Save(cfg); err != nil {
		return cfg, err
	}
	fmt.Printf("\nSaved %s\n", config.Path())
	return cfg, nil
}

func prompt(in *bufio.Reader, label string) string {
	fmt.Print(label)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}
