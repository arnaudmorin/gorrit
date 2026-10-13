package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

// hunkContext is the agent-context sidecar format read by `hunk --agent-context`.
type hunkContext struct {
	Version int        `json:"version"`
	Summary string     `json:"summary,omitempty"`
	Files   []hunkFile `json:"files"`
}

type hunkFile struct {
	Path        string           `json:"path"`
	Annotations []hunkAnnotation `json:"annotations"`
}

type hunkAnnotation struct {
	ID        string   `json:"id,omitempty"`
	OldRange  []int    `json:"oldRange,omitempty"`
	NewRange  []int    `json:"newRange,omitempty"`
	Summary   string   `json:"summary"`
	Rationale string   `json:"rationale,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source,omitempty"`
	Author    string   `json:"author,omitempty"`
	CreatedAt string   `json:"createdAt,omitempty"`
}

// hunkNotes converts the comment threads of one patch set into a hunk
// sidecar. Each thread becomes one annotation whose rationale holds the
// whole conversation.
func hunkNotes(change *gerrit.ChangeInfo, byPath map[string][]gerrit.CommentInfo, patchSet int) hunkContext {
	ctx := hunkContext{Version: 1, Summary: fmt.Sprintf("Gerrit change %d, patch set %d", change.Number, patchSet)}
	files := map[string]*hunkFile{}
	var order []string
	for _, t := range threads(byPath) {
		root := t.root()
		if root.PatchSet != patchSet || magicPaths[t.path] {
			continue
		}
		f, ok := files[t.path]
		if !ok {
			f = &hunkFile{Path: t.path}
			files[t.path] = f
			order = append(order, t.path)
		}
		f.Annotations = append(f.Annotations, threadAnnotation(t, change))
	}
	for _, p := range order {
		ctx.Files = append(ctx.Files, *files[p])
	}
	return ctx
}

func threadAnnotation(t thread, change *gerrit.ChangeInfo) hunkAnnotation {
	root := t.root()
	a := hunkAnnotation{
		ID:        root.ID,
		Source:    "gerrit",
		Author:    commentAuthor(root),
		CreatedAt: root.Updated.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if r := lineRange(root); r != nil {
		if root.Side == "PARENT" {
			a.OldRange = r
		} else {
			a.NewRange = r
		}
	}
	if t.unresolved() {
		a.Tags = []string{"unresolved"}
	} else {
		a.Tags = []string{"resolved"}
	}
	first := strings.TrimSpace(resolvePlaceholders(root.Message, change))
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i] + " …"
	}
	a.Summary = commentAuthor(root) + ": " + first
	var parts []string
	for _, c := range t.comments {
		parts = append(parts, fmt.Sprintf("%s (%s):\n%s", commentAuthor(c), ageText(c.Updated.Time),
			strings.TrimSpace(resolvePlaceholders(c.Message, change))))
	}
	a.Rationale = strings.Join(parts, "\n\n")
	return a
}

func lineRange(c gerrit.CommentInfo) []int {
	switch {
	case c.Range != nil && c.Range.StartLine > 0:
		return []int{c.Range.StartLine, max(c.Range.EndLine, c.Range.StartLine)}
	case c.Line > 0:
		return []int{c.Line, c.Line}
	}
	return nil
}

func marshalNotes(ctx hunkContext) ([]byte, error) {
	return json.MarshalIndent(ctx, "", "  ")
}
