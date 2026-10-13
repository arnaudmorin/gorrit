package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestBugHeaderLive(t *testing.T) {
	url, dir := os.Getenv("GORRIT_TEST_URL"), os.Getenv("GORRIT_SNAPSHOT_DIR")
	if url == "" || dir == "" {
		t.Skip("GORRIT_TEST_URL or GORRIT_SNAPSHOT_DIR not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	client := gerrit.New(url, "", "")
	results, err := client.QueryChanges(context.Background(), []string{"is:open project:openstack/nova message:Closes-Bug"}, gerrit.QueryOptions{Limit: 1})
	if err != nil || len(results[0]) == 0 {
		t.Skipf("no change found: %v", err)
	}
	c := results[0][0]
	change, err := client.GetChangeDetail(context.Background(), gerrit.ChangeID(c.Project, c.Number), detailOptions...)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := renderChange(change, nil, 150, renderOpts{selected: -1, bugURL: config.DefaultBugURL})
	lines := strings.Split(content, "\n")
	os.WriteFile(filepath.Join(dir, "bugs.ansi"), []byte(strings.Join(lines[:min(8, len(lines))], "\n")), 0o644)
	t.Logf("bugs: %v", changeBugs(change))
}
