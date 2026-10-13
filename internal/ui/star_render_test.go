package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestStarColumnRender(t *testing.T) {
	dir := os.Getenv("GORRIT_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("GORRIT_SNAPSHOT_DIR not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	m := newList(gerrit.New("https://example.org", "", ""), config.AnonymousFilters)
	m.width, m.height = 140, 10
	m.selfID = 7
	attention := map[string]gerrit.AttentionSetInfo{"7": {}}
	m, _ = m.Update(listLoadedMsg{query: m.fullQuery(), changes: []gerrit.ChangeInfo{
		{Number: 1, Subject: "starred and attention", Starred: true, AttentionSet: attention, Owner: gerrit.AccountInfo{Name: "A"}, Project: "p", Branch: "main"},
		{Number: 2, Subject: "starred only", Starred: true, Owner: gerrit.AccountInfo{Name: "B"}, Project: "p", Branch: "main"},
		{Number: 3, Subject: "plain", Owner: gerrit.AccountInfo{Name: "C"}, Project: "p", Branch: "main"},
	}})
	m.cursor = 1
	os.WriteFile(filepath.Join(dir, "stars.ansi"), []byte(m.View()), 0o644)
}
