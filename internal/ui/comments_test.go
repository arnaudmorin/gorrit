package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestCommentsLive(t *testing.T) {
	url, dir := os.Getenv("GORRIT_TEST_URL"), os.Getenv("GORRIT_SNAPSHOT_DIR")
	if url == "" {
		t.Skip("GORRIT_TEST_URL not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	client := gerrit.New(url, "", "")
	results, err := client.QueryChanges(context.Background(), []string{"is:open project:openstack/neutron"}, gerrit.QueryOptions{Limit: 30})
	if err != nil {
		t.Fatal(err)
	}
	var change *gerrit.ChangeInfo
	var comments map[string][]gerrit.CommentInfo
	var id string
	for i := range results[0] {
		cand := &results[0][i]
		if cand.TotalCommentCount == 0 {
			continue
		}
		id = gerrit.ChangeID(cand.Project, cand.Number)
		ch, err := client.GetChange(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := client.ListComments(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, th := range threads(cs) {
			if th.root().PatchSet == ch.CurrentRevisionNumber && th.root().Line > 0 {
				change, comments = ch, cs
			}
		}
		if change != nil {
			break
		}
	}
	if change == nil {
		t.Skip("no change with line comments on its current patch set")
	}
	t.Logf("change %d PS%d: %d files with comments", change.Number, change.CurrentRevisionNumber, len(comments))
	notes := hunkNotes(change, comments, change.CurrentRevisionNumber)
	raw, _ := marshalNotes(notes)
	t.Logf("notes: %d files on current PS\n%.1500s", len(notes.Files), raw)
	if dir != "" {
		section, _ := renderComments(threads(comments), 150, change, 1, 0)
		os.WriteFile(filepath.Join(dir, "comments.ansi"), []byte(section), 0o644)

		raw, err := client.GetPatch(context.Background(), id, gerrit.RevisionCurrent)
		if err != nil {
			t.Fatal(err)
		}
		ts := threads(comments)
		sel := -1
		for i, th := range ts {
			if th.root().PatchSet == change.CurrentRevisionNumber && th.root().Line > 0 {
				sel = i
				break
			}
		}
		r := renderDiff(parsePatch(raw), ts, change, 150, sel, change.CurrentRevisionNumber, -1)
		page, positions := r.content, r.threadAt
		t.Logf("diff page: %d lines, %d threads placed, selected %d at line %d", strings.Count(page, "\n"), len(positions), sel, positions[sel])
		lines := strings.Split(page, "\n")
		from := max(positions[sel]-25, 0)
		os.WriteFile(filepath.Join(dir, "diffpage.ansi"), []byte(strings.Join(lines[from:min(from+45, len(lines))], "\n")), 0o644)
	}
}
