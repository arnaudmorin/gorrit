package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

// thread is a root inline comment followed by its replies in time order.
type thread struct {
	path     string
	comments []gerrit.CommentInfo
}

func (t thread) root() gerrit.CommentInfo { return t.comments[0] }

func (t thread) unresolved() bool { return t.comments[len(t.comments)-1].Unresolved }

// threads groups comments into threads, sorted by path, patch set and line.
func threads(byPath map[string][]gerrit.CommentInfo) []thread {
	var out []thread
	for _, path := range sortedKeys(byPath) {
		comments := byPath[path]
		byID := map[string]gerrit.CommentInfo{}
		for _, c := range comments {
			byID[c.ID] = c
		}
		rootOf := func(c gerrit.CommentInfo) string {
			for c.InReplyTo != "" {
				parent, ok := byID[c.InReplyTo]
				if !ok {
					break
				}
				c = parent
			}
			return c.ID
		}
		grouped := map[string][]gerrit.CommentInfo{}
		var order []string
		for _, c := range comments {
			r := rootOf(c)
			if _, seen := grouped[r]; !seen {
				order = append(order, r)
			}
			grouped[r] = append(grouped[r], c)
		}
		for _, r := range order {
			cs := grouped[r]
			sort.SliceStable(cs, func(i, j int) bool { return cs[i].Updated.Before(cs[j].Updated.Time) })
			out = append(out, thread{path: path, comments: cs})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].root(), out[j].root()
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		if a.PatchSet != b.PatchSet {
			return a.PatchSet < b.PatchSet
		}
		return a.Line < b.Line
	})
	return out
}

func commentLocation(c gerrit.CommentInfo) string {
	switch {
	case c.Range != nil && c.Range.StartLine != c.Range.EndLine:
		return fmt.Sprintf("L%d-%d", c.Range.StartLine, c.Range.EndLine)
	case c.Line > 0:
		return fmt.Sprintf("L%d", c.Line)
	}
	return "file"
}

func commentAuthor(c gerrit.CommentInfo) string {
	if c.Author != nil {
		return c.Author.Label()
	}
	return "unknown"
}

// replyInput builds the comment that answers a thread, anchored like its root.
func (t thread) replyInput(message string, unresolved bool) gerrit.CommentInput {
	root, last := t.root(), t.comments[len(t.comments)-1]
	return gerrit.CommentInput{
		Path:       t.path,
		Side:       root.Side,
		Line:       root.Line,
		Range:      root.Range,
		InReplyTo:  last.ID,
		Message:    message,
		Unresolved: &unresolved,
	}
}

// lineWriter accumulates content while counting lines, so renderers can
// report where notable items landed.
type lineWriter struct {
	b     strings.Builder
	lines int
}

func (w *lineWriter) writeln(s string) {
	w.b.WriteString(s + "\n")
	w.lines += strings.Count(s, "\n") + 1
}

func (w *lineWriter) String() string { return w.b.String() }

// renderThread writes one thread; withContext adds the source lines Gerrit
// returned around the root comment.
func renderThread(w *lineWriter, t thread, selected bool, change *gerrit.ChangeInfo, width int, withContext bool) {
	root := t.root()
	mark := styleGreen.Render("✔")
	if t.unresolved() {
		mark = styleRed.Render("●")
	}
	side := ""
	if root.Side == "PARENT" {
		side = " base"
	}
	gutter := "  "
	header := styleYellow.Render(commentLocation(root)+side) + styleMuted.Render(fmt.Sprintf(" · PS%d", root.PatchSet))
	if selected {
		gutter = styleAccent.Render("▌ ")
		header = lipgloss.NewStyle().Bold(true).Render(header) + styleMuted.Render("  c reply · x done")
	}
	w.writeln(gutter + mark + " " + header)
	if withContext {
		for _, cl := range root.Context {
			num := styleMuted.Render(fmt.Sprintf("%5d", cl.LineNumber))
			text := styleMuted.Render(ansi.Truncate(expandTabs(cl.Text), width-12, "…"))
			if inRange(root, cl.LineNumber) {
				num = styleYellow.Render(fmt.Sprintf("%5d", cl.LineNumber))
				text = styleText.Render(ansi.Truncate(expandTabs(cl.Text), width-12, "…"))
			}
			w.writeln("    " + num + styleMuted.Render(" │ ") + text)
		}
	}
	for _, c := range t.comments {
		w.writeln("    " + styleOwner.Render(commentAuthor(c)) + styleMuted.Render("  "+ageText(c.Updated.Time)))
		for _, line := range strings.Split(strings.TrimSpace(resolvePlaceholders(c.Message, change)), "\n") {
			for _, wrapped := range strings.Split(wrap(line, width-6), "\n") {
				w.writeln(styleMuted.Render("    │ ") + wrapped)
			}
		}
	}
}

func displayPath(path string) string {
	if path == "/PATCHSET_LEVEL" {
		return "Patch set comments"
	}
	return path
}

// renderComments writes all threads grouped by file and returns the content
// line of each thread header, offset by base, keyed by thread index.
func renderComments(ts []thread, width int, change *gerrit.ChangeInfo, selected, base int) (string, map[int]int) {
	if len(ts) == 0 {
		return "", nil
	}
	unresolved := 0
	for _, t := range ts {
		if t.unresolved() {
			unresolved++
		}
	}
	w := &lineWriter{}
	title := fmt.Sprintf("Inline comments (%d threads", len(ts))
	if unresolved > 0 {
		title += fmt.Sprintf(", %d unresolved", unresolved)
	}
	w.writeln(rule(title+")", width))
	positions := map[int]int{}
	lastPath := ""
	for i, t := range ts {
		if t.path != lastPath {
			lastPath = t.path
			w.writeln("  " + styleAccent.Bold(true).Render(displayPath(t.path)))
		}
		positions[i] = base + w.lines
		renderThread(w, t, i == selected, change, width, true)
	}
	return w.String(), positions
}

func inRange(c gerrit.CommentInfo, line int) bool {
	if c.Range != nil {
		return line >= c.Range.StartLine && line <= c.Range.EndLine
	}
	return line == c.Line
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}
