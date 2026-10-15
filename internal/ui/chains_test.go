package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestChainsInList(t *testing.T) {
	chain := `{"changes":[{"_change_number":12},{"_change_number":11},{"_change_number":10},{"_change_number":9}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body := `{"changes":[]}`
		if !strings.Contains(req.URL.Path, "~20/") {
			body = chain
		}
		w.Write([]byte(")]}'\n" + body))
	}))
	defer srv.Close()
	stamp := gerrit.Timestamp{Time: time.Unix(1000, 0)}
	changes := []gerrit.ChangeInfo{
		{Project: "p", Number: 12, Subject: "top", Updated: stamp},
		{Project: "p", Number: 20, Subject: "alone", Updated: stamp},
		{Project: "p", Number: 10, Subject: "bottom", Updated: stamp},
	}
	m := newList(gerrit.New(srv.URL, "", ""), config.AnonymousFilters)
	m.width, m.height = 200, 10
	m, cmd := m.Update(listLoadedMsg{query: m.fullQuery(), changes: changes})
	if cmd == nil {
		t.Fatal("no chain lookup")
	}
	m, _ = m.Update(cmd())
	want := map[int]chainPos{12: {base: 9, index: 4, size: 4}, 10: {base: 9, index: 2, size: 4}, 20: {}}
	for n, p := range want {
		if got := m.chains[n].pos; got != p {
			t.Fatalf("change %d: got %+v want %+v", n, got, p)
		}
	}
	view := ansiStrip(m.View())
	if !strings.Contains(view, "4/4 · top") || !strings.Contains(view, "2/4 · bottom") || strings.Contains(view, "/4 alone") {
		t.Fatalf("view:\n%s", view)
	}
	if _, cmd := m.Update(listLoadedMsg{query: m.fullQuery(), changes: changes}); cmd != nil {
		t.Fatal("unchanged changes looked up again")
	}
}
