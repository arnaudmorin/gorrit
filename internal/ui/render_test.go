package ui

import (
	"context"
	"os"
	"testing"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestRenderAgainstLiveServer(t *testing.T) {
	url := os.Getenv("GORRIT_TEST_URL")
	if url == "" {
		t.Skip("GORRIT_TEST_URL not set")
	}
	client := gerrit.New(url, os.Getenv("GORRIT_USERNAME"), os.Getenv("GORRIT_TOKEN"))

	dash := newDashboard(client, nil)
	dash.width, dash.height = 160, 30
	msg := dash.load()()
	loaded := msg.(dashboardLoadedMsg)
	if loaded.err != nil {
		t.Fatal(loaded.err)
	}
	dash, _ = dash.Update(loaded)
	t.Log("\n" + dash.View())

	sel := dash.selected()
	if sel == nil {
		t.Fatal("no change selected")
	}
	change, err := client.GetChangeDetail(context.Background(), gerrit.ChangeID(sel.Project, sel.Number), detailOptions...)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := renderChange(change, nil, 120, renderOpts{selected: -1, showAutomated: true, bugURL: config.DefaultBugURL})
	t.Log("\n" + content)
}
