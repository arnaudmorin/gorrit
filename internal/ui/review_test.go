package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestReviewRunSaveAndTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/patch") {
			w.Write([]byte("ZGlmZiAtLWdpdCBhL2YgYi9mCg=="))
			return
		}
		w.Write([]byte(")]}'\n{\"current_revision_number\":3}"))
	}))
	defer srv.Close()
	client := gerrit.New(srv.URL, "", "")
	id := gerrit.ChangeID("p", 1)
	plain := func(s lipgloss.Style) lipgloss.Style { return s }

	s := newReviewStore("claude", "grep -c diff", t.TempDir())
	if cmd := s.start(client, id); cmd == nil {
		t.Fatal("no command")
	}
	if s.start(client, id) != nil {
		t.Fatal("second start while running")
	}
	if got := ansiStrip(s.listTag(id, 3, plain)); got != "⠋ claude" {
		t.Fatalf("running tag %q", got)
	}
	msg := runReview(client, id, s.command)().(reviewDoneMsg)
	if msg.err != nil || msg.patchSet != 3 || strings.TrimSpace(msg.output) != "1" {
		t.Fatalf("got %+v", msg)
	}
	if err := s.finish(msg); err != nil {
		t.Fatal(err)
	}
	if got := ansiStrip(s.listTag(id, 3, plain)); got != "✓ claude" {
		t.Fatalf("done tag %q", got)
	}
	if got := ansiStrip(s.listTag(id, 4, plain)); got != "✓ claude ps3" {
		t.Fatalf("stale tag %q", got)
	}

	reloaded := newReviewStore("claude", "", s.dir)
	if r := reloaded.byID[id]; r == nil || r.patchSet != 3 || r.output != "1" {
		t.Fatalf("reloaded %+v", r)
	}
	if !strings.Contains(ansiStrip(reloaded.section(id, 3, 80)), "Claude review · patch set 3") {
		t.Fatal(reloaded.section(id, 3, 80))
	}

	failed := runReview(client, id, "echo boom >&2; exit 2")().(reviewDoneMsg)
	if failed.err == nil || !strings.Contains(failed.err.Error(), "boom") {
		t.Fatalf("got %+v", failed)
	}
	s.finish(failed)
	if got := ansiStrip(s.listTag(id, 3, plain)); got != "✗ claude" {
		t.Fatalf("failed tag %q", got)
	}
}

func TestParseNotes(t *testing.T) {
	out := "Looks fine overall:1 nit.\n\n" +
		"f.py:11: renamed without a caller update\n  callers still use the old name\n\n" +
		"1. `b/gone.txt:1` — removed file is still referenced\n" +
		"- other.py:3, 7 - unrelated\n\nThanks."
	got := parseNotes(out)
	want := []reviewNote{
		{path: "f.py", line: 11, text: "renamed without a caller update\ncallers still use the old name"},
		{path: "gone.txt", line: 1, text: "removed file is still referenced"},
		{path: "other.py", line: 3, text: "unrelated"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("note %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestDiffShowsReviewNotes(t *testing.T) {
	notes := reviewNotes{name: "claude", items: []reviewNote{
		{path: "f.py", line: 11, text: "check this"},
		{path: "f.py", line: 500, text: "far away"},
		{path: "other.py", line: 3, text: "not in diff"},
	}}
	content := ansiStrip(renderDiff(parsePatch([]byte(samplePatch)), nil, notes, &gerrit.ChangeInfo{}, 100, -1, 1, 0, -1).content)
	lines := strings.Split(content, "\n")
	at := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		t.Fatalf("%q missing in:\n%s", s, content)
		return -1
	}
	if at("check this") <= at("11 +") {
		t.Fatalf("note not under its line:\n%s", content)
	}
	at("claude notes outside the diff context")
	if at("not in diff") <= at("claude notes on files not in this diff") {
		t.Fatalf("stray note misplaced:\n%s", content)
	}
}

func TestVerdict(t *testing.T) {
	for out, want := range map[string]string{
		"Good change.\nVerdict: +1\n\nf.py:1: nit": "+1",
		"**Verdict: -1** — tests missing":          "-1",
		"verdict -1":                               "-1",
		"No verdict here, +1 maybe":                "",
	} {
		if got := verdict(out); got != want {
			t.Fatalf("%q: got %q want %q", out, got, want)
		}
	}
	plain := func(s lipgloss.Style) lipgloss.Style { return s }
	s := newReviewStore("claude", "", "")
	s.byID["p~1"] = &aiReview{state: reviewDone, patchSet: 2, output: "Fine.\nVerdict: +1"}
	if got := ansiStrip(s.listTag("p~1", 2, plain)); got != "✓ claude +1" {
		t.Fatalf("tag %q", got)
	}
}
