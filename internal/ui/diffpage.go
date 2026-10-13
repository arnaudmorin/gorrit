package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

// lineAnchor is a comment position: a line of a file on one side of the diff.
type lineAnchor struct {
	path string
	side string // REVISION or PARENT
	line int    // 0 for a file-level comment
}

func threadAnchor(t thread) lineAnchor {
	root := t.root()
	side := root.Side
	if side == "" {
		side = "REVISION"
	}
	return lineAnchor{path: t.path, side: side, line: root.Line}
}

func (a lineAnchor) String() string {
	switch {
	case a.line == 0:
		return a.path + " (file comment)"
	case a.side == "PARENT":
		return fmt.Sprintf("%s old line %d", a.path, a.line)
	}
	return fmt.Sprintf("%s line %d", a.path, a.line)
}

type diffRender struct {
	content  string
	threadAt map[int]int        // thread index → content line
	anchors  map[int]lineAnchor // content line → comment target
	lines    []int              // content lines that take the cursor, in order
}

// renderDiff writes the unified diff of the current patch set with the
// threads of that patch set inlined under their lines. cursor is the content
// line to highlight.
func renderDiff(files []diffFile, ts []thread, change *gerrit.ChangeInfo, width, selected, patchSet, cursor int) diffRender {
	w := &lineWriter{}
	positions := map[int]int{}
	out := diffRender{threadAt: positions, anchors: map[int]lineAnchor{}}
	mark := func(a lineAnchor) {
		out.anchors[w.lines] = a
		out.lines = append(out.lines, w.lines)
	}
	byAnchor := map[lineAnchor][]int{}
	byPath := map[string][]int{}
	placed := map[int]bool{}
	for i, t := range ts {
		if t.root().PatchSet != patchSet {
			continue
		}
		byPath[t.path] = append(byPath[t.path], i)
		if t.root().Line > 0 {
			byAnchor[threadAnchor(t)] = append(byAnchor[threadAnchor(t)], i)
		}
	}
	emit := func(idx []int) {
		for _, i := range idx {
			if placed[i] {
				continue
			}
			placed[i] = true
			positions[i] = w.lines
			renderThread(w, ts[i], i == selected, change, width, false)
		}
	}

	older := 0
	for _, t := range ts {
		if t.root().PatchSet != patchSet {
			older++
		}
	}
	if older > 0 {
		w.writeln(styleMuted.Render(fmt.Sprintf("  %d threads on older patch sets are only listed on the summary page", older)))
	}
	if idx := byPath["/PATCHSET_LEVEL"]; len(idx) > 0 {
		w.writeln(rule("Patch set comments", width))
		emit(idx)
		w.writeln("")
	}

	gutterWidth := 13
	for _, f := range files {
		name := f.path
		if f.status == "R" {
			name = styleMuted.Render(f.oldPath+" → ") + f.path
		}
		mark(lineAnchor{path: f.path})
		header := rule(fileStatus(f.status)+" "+name, width)
		if w.lines == cursor {
			header = styleAccent.Render("▌") + header
		}
		w.writeln(header)
		for _, i := range byPath[f.path] {
			if ts[i].root().Line == 0 {
				emit([]int{i})
			}
		}
		if f.binary {
			w.writeln(styleMuted.Render("  binary file"))
		}
		for _, l := range f.lines {
			switch l.kind {
			case '@':
				w.writeln(styleAccent.Render(ansi.Truncate(l.text, width, "…")))
				continue
			case 'm':
				w.writeln(styleMuted.Render(l.text))
				continue
			}
			oldNum, newNum := "     ", "     "
			if l.old > 0 {
				oldNum = fmt.Sprintf("%5d", l.old)
			}
			if l.new > 0 {
				newNum = fmt.Sprintf("%5d", l.new)
			}
			if l.kind == '-' {
				mark(lineAnchor{path: f.path, side: "PARENT", line: l.old})
			} else {
				mark(lineAnchor{path: f.path, side: "REVISION", line: l.new})
			}
			sel := w.lines == cursor
			st := func(base lipgloss.Style) lipgloss.Style { return onRow(base, sel) }
			text := ansi.Truncate(expandTabs(l.text), max(width-gutterWidth, 10), "…")
			var line string
			switch l.kind {
			case '+':
				line = st(styleMuted).Render(oldNum+" "+newNum) + st(styleGreen).Render(" +"+text)
			case '-':
				line = st(styleMuted).Render(oldNum+" "+newNum) + st(styleRed).Render(" -"+text)
			default:
				line = st(styleMuted).Render(oldNum+" "+newNum+"  ") + st(styleText).Render(text)
			}
			if sel {
				if fill := width - ansi.StringWidth(line); fill > 0 {
					line += st(lipgloss.NewStyle()).Render(strings.Repeat(" ", fill))
				}
			}
			w.writeln(line)
			if l.new > 0 {
				emit(byAnchor[lineAnchor{f.path, "REVISION", l.new}])
			}
			if l.old > 0 {
				emit(byAnchor[lineAnchor{f.path, "PARENT", l.old}])
			}
		}
		var leftover []int
		for _, i := range byPath[f.path] {
			if !placed[i] {
				leftover = append(leftover, i)
			}
		}
		if len(leftover) > 0 {
			w.writeln(styleMuted.Render("  comments outside the diff context:"))
			emit(leftover)
		}
		w.writeln("")
	}
	for i := range ts {
		if ts[i].root().PatchSet == patchSet && !placed[i] && !strings.HasPrefix(ts[i].path, "/") {
			w.writeln(rule("Comments on files not in this diff", width))
			break
		}
	}
	for i := range ts {
		if ts[i].root().PatchSet == patchSet && !placed[i] {
			w.writeln("  " + styleAccent.Bold(true).Render(displayPath(ts[i].path)))
			emit([]int{i})
		}
	}
	out.content = w.String()
	return out
}
