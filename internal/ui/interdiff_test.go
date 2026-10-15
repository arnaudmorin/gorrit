package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i+1)
	}
	return out
}

func TestFileFromDiffHunks(t *testing.T) {
	d := &gerrit.DiffInfo{Content: []gerrit.DiffContent{
		{AB: numbered("a", 10)},
		{A: []string{"old"}, B: []string{"new", "new2"}},
		{AB: numbered("b", 10)},
		{Skip: 100},
		{AB: numbered("c", 2)},
		{B: []string{"tail"}},
	}}
	f := fileFromDiff("f.py", gerrit.FileInfo{}, d)
	var headers []string
	for _, l := range f.lines {
		if l.kind == '@' {
			headers = append(headers, l.text)
		}
	}
	want := []string{"@@ -8,7 +8,8 @@", "@@ -122,2 +123,3 @@"}
	if strings.Join(headers, "|") != strings.Join(want, "|") {
		t.Fatalf("headers %v", headers)
	}
	if f.status != "M" || f.oldPath != "f.py" {
		t.Fatalf("file %+v", f)
	}
	last := f.lines[len(f.lines)-1]
	if last.kind != '+' || last.new != 125 || last.text != "tail" {
		t.Fatalf("last line %+v", last)
	}
}

func TestBracketsStepThroughPatchSets(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits = append(hits, req.URL.EscapedPath()+"?"+req.URL.RawQuery)
		switch {
		case strings.HasSuffix(req.URL.Path, "/patch"):
			w.Write([]byte("ZGlmZiAtLWdpdCBhL2YgYi9mCg=="))
		case strings.HasSuffix(req.URL.Path, "/files/"):
			w.Write([]byte(")]}'\n{\"/COMMIT_MSG\":{},\"f\":{}}"))
		default:
			w.Write([]byte(")]}'\n{\"content\":[{\"a\":[\"x\"],\"b\":[\"y\"]}]}"))
		}
	}))
	defer srv.Close()
	m := newDetail(gerrit.New(srv.URL, "", ""), "", nil)
	m.id = gerrit.ChangeID("p", 1)
	m.change = &gerrit.ChangeInfo{CurrentRevisionNumber: 3}
	m.width, m.height = 100, 30
	m.layout()

	press := func(k string) {
		var cmd tea.Cmd
		m, cmd = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		if cmd != nil {
			m, _ = m.Update(cmd())
		}
	}
	press("]")
	if m.page != pageDiff || m.diffBase != 0 {
		t.Fatalf("page %v base %d", m.page, m.diffBase)
	}
	press("]")
	if m.diffBase != 2 || len(m.files) != 1 || m.files[0].path != "f" {
		t.Fatalf("base %d files %+v", m.diffBase, m.files)
	}
	press("]")
	press("]")
	if m.diffBase != 1 {
		t.Fatalf("base %d after stepping past the oldest", m.diffBase)
	}
	press("[")
	press("[")
	press("[")
	if m.page != pageSummary {
		t.Fatalf("page %v", m.page)
	}
	press("]")
	press("]")
	want := []string{
		"/changes/p~1/revisions/current/patch?",
		"/changes/p~1/revisions/current/files/?base=2",
		"/changes/p~1/revisions/current/files/f/diff?base=2",
		"/changes/p~1/revisions/current/files/?base=1",
		"/changes/p~1/revisions/current/files/f/diff?base=1",
	}
	if strings.Join(hits, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(hits, "\n"))
	}
}
