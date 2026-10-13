package ui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestDashboardViewBeforeResize(t *testing.T) {
	m := newDashboard(gerrit.New("https://example.org", "", ""), nil)
	for _, size := range [][2]int{{0, 0}, {80, 1}, {80, 2}, {80, 3}, {20, 5}} {
		m.width, m.height = size[0], size[1]
		_ = m.View()
		m, _ = m.Update(dashboardLoadedMsg{sections: []section{{title: "s", query: "q", changes: []gerrit.ChangeInfo{{Number: 1, Subject: "x"}, {Number: 2}}}}})
		m.cursor = 1
		_ = m.View()
	}
}

func TestVoteCellShowsBothExtremes(t *testing.T) {
	label := gerrit.LabelInfo{All: []gerrit.ApprovalInfo{{Value: 2}, {Value: 1}, {Value: -1}}}
	got := ansiStrip(voteCell(label, 4, false))
	if got != "+2-1" {
		t.Fatalf("got %q", got)
	}
	label.Approved = &gerrit.AccountInfo{}
	if got := ansiStrip(voteCell(label, 4, false)); got != "+2-1" {
		t.Fatalf("got %q", got)
	}
}

func ansiStrip(s string) string { return ansi.Strip(s) }

func TestLoadMoreLive(t *testing.T) {
	url := os.Getenv("GORRIT_TEST_URL")
	if url == "" {
		t.Skip("GORRIT_TEST_URL not set")
	}
	m := newDashboard(gerrit.New(url, "", ""), nil)
	m.width, m.height = 160, 40
	m, _ = m.Update(m.load()())
	first := len(m.sections[0].changes)
	if first == 0 || !m.sections[0].changes[first-1].MoreChanges {
		t.Skip("first page not full")
	}
	m.cursor = first - 1
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd == nil {
		t.Fatal("expected a load-more command")
	}
	m, _ = m.Update(cmd())
	if got := len(m.sections[0].changes); got <= first {
		t.Fatalf("got %d changes after paging, had %d", got, first)
	}
	if m.sections[0].changes[first-1].MoreChanges {
		t.Fatal("page boundary still flagged as more")
	}
	t.Logf("paged from %d to %d changes", first, len(m.sections[0].changes))
}

func TestScrollDoesNotSnapBack(t *testing.T) {
	m := newDashboard(gerrit.New("https://example.org", "", ""), nil)
	m.width, m.height = 120, 12
	var changes []gerrit.ChangeInfo
	for i := 0; i < 40; i++ {
		changes = append(changes, gerrit.ChangeInfo{Number: i + 1, Subject: "x"})
	}
	m, _ = m.Update(dashboardLoadedMsg{sections: []section{{title: "s", query: "q", changes: changes}}})
	key := func(r rune) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
	for i := 0; i < 30; i++ {
		key('j')
	}
	down := m.offset
	if down == 0 {
		t.Fatal("expected the view to have scrolled")
	}
	for i := 0; i < 3; i++ {
		key('k')
	}
	if m.offset != down {
		t.Fatalf("offset moved from %d to %d when cursor was still visible", down, m.offset)
	}
	for i := 0; i < 30; i++ {
		key('k')
	}
	if m.offset != 0 {
		t.Fatalf("offset %d after returning to the top", m.offset)
	}
}
