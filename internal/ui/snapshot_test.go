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

// TestSnapshot writes ANSI renderings of both views to GORRIT_SNAPSHOT_DIR
// using live data from GORRIT_TEST_URL.
func TestSnapshot(t *testing.T) {
	url, dir := os.Getenv("GORRIT_TEST_URL"), os.Getenv("GORRIT_SNAPSHOT_DIR")
	if url == "" || dir == "" {
		t.Skip("GORRIT_TEST_URL or GORRIT_SNAPSHOT_DIR not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	client := gerrit.New(url, os.Getenv("GORRIT_USERNAME"), os.Getenv("GORRIT_TOKEN"))
	app := New(client, Options{Filters: append(config.AnonymousFilters, config.Filter{Name: "Nova", Query: "project:openstack/nova"}), BugURL: config.DefaultBugURL})
	app.list.width, app.list.height = 150, 36
	app.detail.resize(150, 36)

	msg := app.list.refresh()()
	if m := msg.(listLoadedMsg); m.err != nil {
		t.Fatal(m.err)
	}
	app.list, _ = app.list.Update(msg)
	app.list.cursor = 2
	app.list.status = statusAll
	if sel := app.list.selected(); sel != nil {
		app.list.selfID = sel.Owner.AccountID
	}
	write(t, filepath.Join(dir, "list.ansi"), app.list.View())

	sel := app.list.selected()
	change, err := client.GetChangeDetail(context.Background(), gerrit.ChangeID(sel.Project, sel.Number), detailOptions...)
	if err != nil {
		t.Fatal(err)
	}
	app.detail.id = gerrit.ChangeID(sel.Project, sel.Number)
	comments, err := client.ListComments(context.Background(), app.detail.id)
	if err != nil {
		t.Fatal(err)
	}
	app.detail, _ = app.detail.Update(detailLoadedMsg{id: app.detail.id, change: change, comments: comments})
	write(t, filepath.Join(dir, "detail.ansi"), app.detail.View())
	app.detail.download = newDownload(change.CurrentRevisionInfo().Fetch)
	write(t, filepath.Join(dir, "download.ansi"), app.detail.View())
	app.detail.download.open = false
	app.detail.viewport.GotoBottom()
	app.detail.viewport.SetYOffset(app.detail.viewport.YOffset - 60)
	write(t, filepath.Join(dir, "detail-messages.ansi"), app.detail.View())
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
