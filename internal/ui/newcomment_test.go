package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestDiffAnchorsAndNewComment(t *testing.T) {
	files := parsePatch([]byte(samplePatch))
	r := renderDiff(files, nil, &gerrit.ChangeInfo{}, 100, -1, 1, -1)
	var got []lineAnchor
	for _, idx := range r.lines {
		got = append(got, r.anchors[idx])
	}
	want := []lineAnchor{
		{path: "f.py"},
		{path: "f.py", side: "REVISION", line: 10},
		{path: "f.py", side: "PARENT", line: 11},
		{path: "f.py", side: "REVISION", line: 11},
		{path: "f.py", side: "REVISION", line: 12},
		{path: "f.py", side: "REVISION", line: 13},
		{path: "gone.txt"},
		{path: "gone.txt", side: "PARENT", line: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("anchor %d: got %+v want %+v", i, got[i], want[i])
		}
	}

	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path = req.URL.EscapedPath()
		raw, _ := io.ReadAll(req.Body)
		json.Unmarshal(raw, &body)
		w.Write([]byte(")]}'\n{}"))
	}))
	defer srv.Close()
	m := newDetail(gerrit.New(srv.URL, "alice", "tok"), "", nil)
	m.id = gerrit.ChangeID("p", 1)
	m.change = &gerrit.ChangeInfo{CurrentRevisionNumber: 3}
	m.sendNewComment(want[2], "why?", true)()
	if path != "/a/changes/p~1/revisions/3/review" {
		t.Fatal(path)
	}
	c := body["comments"].(map[string]any)["f.py"].([]any)[0].(map[string]any)
	if c["line"] != float64(11) || c["side"] != "PARENT" || c["message"] != "why?" || c["unresolved"] != true {
		t.Fatalf("payload %v", c)
	}

	if dir := os.Getenv("GORRIT_SNAPSHOT_DIR"); dir != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		m.width, m.height = 120, 20
		m.layout()
		m.files = files
		m.hasPatch = true
		m.showPage(pageDiff)
		m.moveLine(3)
		m, _ = m.startNewComment(*m.cursorAnchor())
		m.compose.SetValue("Is this intended?")
		view := m.View()
		os.WriteFile(filepath.Join(dir, "newcomment.ansi"), []byte(strings.TrimRight(view, "\n")), 0o644)
	}
}

func TestDiffPagingMovesCursor(t *testing.T) {
	var lines []string
	lines = append(lines, "diff --git a/f.py b/f.py", "--- a/f.py", "+++ b/f.py", "@@ -1,60 +1,60 @@")
	for i := 0; i < 60; i++ {
		lines = append(lines, " line")
	}
	m := newDetail(gerrit.New("https://example.org", "", ""), "", nil)
	m.change = &gerrit.ChangeInfo{CurrentRevisionNumber: 1}
	m.width, m.height = 100, 22
	m.layout()
	m.files = parsePatch([]byte(strings.Join(lines, "\n") + "\n"))
	m.hasPatch = true
	m.showPage(pageDiff)
	start := m.diffLines[m.diffPos]
	m.moveLineBy(m.viewport.Height)
	if got := m.diffLines[m.diffPos] - start; got < m.viewport.Height-2 || got > m.viewport.Height+2 {
		t.Fatalf("cursor moved %d lines for a page of %d", got, m.viewport.Height)
	}
	if m.viewport.YOffset == 0 {
		t.Fatal("view did not follow the cursor")
	}
	m.moveLineBy(-500)
	if m.diffPos != 0 || m.viewport.YOffset != 0 {
		t.Fatalf("expected top, got pos %d offset %d", m.diffPos, m.viewport.YOffset)
	}
}
