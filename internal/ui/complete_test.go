package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestCompleterStatic(t *testing.T) {
	var c completer
	client := gerrit.New("https://example.org", "", "")
	text, cmd := c.complete(client, "is:open proj")
	if cmd != nil || text != "is:open project:" {
		t.Fatalf("got %q", text)
	}
	text, _ = c.complete(client, text)
	if text != "is:open projects:" {
		t.Fatalf("cycle got %q", text)
	}
	text, _ = c.complete(client, text)
	if text != "is:open project:" {
		t.Fatalf("wrap got %q", text)
	}
	text, _ = c.complete(client, "-is:wi")
	if text != "-is:wip" {
		t.Fatalf("negated value got %q", text)
	}
	text, _ = c.complete(client, "(status:m")
	if text != "(status:merged" {
		t.Fatalf("paren got %q", text)
	}
	if text, _ := c.complete(client, "nonsense:x"); text != "nonsense:x" {
		t.Fatalf("unknown op got %q", text)
	}
}

func TestCompleterApplyRejectsStale(t *testing.T) {
	var c completer
	c.pending, c.base = "project:nov", ""
	if _, ok := c.apply(completionsMsg{base: "", word: "project:nov", candidates: []string{"project:openstack/nova"}}, "project:nova-x"); ok {
		t.Fatal("stale completion applied")
	}
	c.pending = "project:nov"
	text, ok := c.apply(completionsMsg{base: "", word: "project:nov", candidates: []string{"project:openstack/nova"}}, "project:nov")
	if !ok || text != "project:openstack/nova" {
		t.Fatalf("got %q %v", text, ok)
	}
}

func TestCompleterLive(t *testing.T) {
	url := os.Getenv("GORRIT_TEST_URL")
	if url == "" {
		t.Skip("GORRIT_TEST_URL not set")
	}
	client := gerrit.New(url, "", "")
	var c completer
	text, cmd := c.complete(client, "is:open project:openstack/neutron-")
	if cmd == nil || text != "is:open project:openstack/neutron-" {
		t.Fatalf("expected a fetch, got %q", text)
	}
	msg := cmd().(completionsMsg)
	text, ok := c.apply(msg, "is:open project:openstack/neutron-")
	t.Logf("projects: %d candidates, first %q", len(msg.candidates), text)
	if !ok || len(msg.candidates) == 0 {
		t.Fatal("no project candidates")
	}
	accounts, err := client.SuggestAccounts(context.Background(), "morin", 5)
	t.Logf("accounts: %d, err %v", len(accounts), err)

	if dir := os.Getenv("GORRIT_SNAPSHOT_DIR"); dir != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		m := newList(client, config.AnonymousFilters)
		m.width, m.height = 140, 8
		m.searching = true
		m.search.Focus()
		m.search.SetValue(text)
		m.complete = c
		os.WriteFile(filepath.Join(dir, "complete.ansi"), []byte(m.View()), 0o644)
	}
}
