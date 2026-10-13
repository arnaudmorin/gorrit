package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func fakeVotableChange() *gerrit.ChangeInfo {
	me := gerrit.AccountInfo{AccountID: 7, Name: "Me"}
	return &gerrit.ChangeInfo{
		Number: 42,
		PermittedLabels: map[string][]string{
			"Code-Review": {"-2", "-1", " 0", "+1", "+2"},
			"Workflow":    {"-1", " 0", "+1"},
		},
		Labels: map[string]gerrit.LabelInfo{
			"Code-Review": {All: []gerrit.ApprovalInfo{{AccountInfo: me, Value: 1}, {Value: 2}}},
			"Workflow":    {All: []gerrit.ApprovalInfo{{AccountInfo: me, Value: 0}}},
		},
	}
}

func TestVotePanel(t *testing.T) {
	v := newVote(fakeVotableChange(), 7)
	if len(v.labels) != 2 || v.labels[0].name != "Code-Review" || v.labels[0].current != 1 {
		t.Fatalf("labels %+v", v.labels)
	}
	key := func(s string) {
		var r []rune
		for _, c := range s {
			r = append(r, c)
		}
		v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: r})
	}
	key("l")
	key("l") // clamps at +2
	key("j")
	key("l")
	if got := v.changes(); got["Code-Review"] != 2 || got["Workflow"] != 1 || len(got) != 2 {
		t.Fatalf("changes %v", got)
	}
	key("h")
	if _, ok := v.changes()["Workflow"]; ok {
		t.Fatal("unchanged vote must not be sent")
	}
	if dir := os.Getenv("GORRIT_SNAPSHOT_DIR"); dir != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		os.WriteFile(filepath.Join(dir, "vote.ansi"), []byte(v.View(120)), 0o644)
	}
}
