package ui

import (
	"strconv"
	"strings"
)

type diffLine struct {
	kind byte // ' ', '+', '-', '@' hunk header, 'm' meta
	old  int
	new  int
	text string
}

type diffFile struct {
	path    string
	oldPath string
	status  string
	binary  bool
	lines   []diffLine
}

// parsePatch reads a git format-patch into files with old and new line
// numbers on every diff line.
func parsePatch(raw []byte) []diffFile {
	var files []diffFile
	var cur *diffFile
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, diffFile{status: "M"})
			cur = &files[len(files)-1]
			inHunk = false
			a, b := splitGitPaths(strings.TrimPrefix(line, "diff --git "))
			cur.oldPath, cur.path = a, b
		case cur == nil:
			continue
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
			oldLine, newLine = hunkStart(line)
			cur.lines = append(cur.lines, diffLine{kind: '@', text: line})
		case !inHunk:
			switch {
			case strings.HasPrefix(line, "new file mode"):
				cur.status = "A"
			case strings.HasPrefix(line, "deleted file mode"):
				cur.status = "D"
			case strings.HasPrefix(line, "rename from "):
				cur.status = "R"
				cur.oldPath = strings.TrimPrefix(line, "rename from ")
			case strings.HasPrefix(line, "rename to "):
				cur.path = strings.TrimPrefix(line, "rename to ")
			case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
				cur.binary = true
			case strings.HasPrefix(line, "--- a/"):
				cur.oldPath = strings.TrimPrefix(line, "--- a/")
			case strings.HasPrefix(line, "+++ b/"):
				cur.path = strings.TrimPrefix(line, "+++ b/")
			}
		case strings.HasPrefix(line, "+"):
			cur.lines = append(cur.lines, diffLine{kind: '+', new: newLine, text: line[1:]})
			newLine++
		case strings.HasPrefix(line, "-"):
			if line == "-- " {
				return files
			}
			cur.lines = append(cur.lines, diffLine{kind: '-', old: oldLine, text: line[1:]})
			oldLine++
		case strings.HasPrefix(line, " "):
			cur.lines = append(cur.lines, diffLine{kind: ' ', old: oldLine, new: newLine, text: line[1:]})
			oldLine++
			newLine++
		case line == "":
			cur.lines = append(cur.lines, diffLine{kind: ' ', old: oldLine, new: newLine})
			oldLine++
			newLine++
		case strings.HasPrefix(line, "\\ "):
			cur.lines = append(cur.lines, diffLine{kind: 'm', text: line})
		}
	}
	return files
}

func splitGitPaths(s string) (string, string) {
	// "a/path b/path"; paths with spaces are rare and resolved by ---/+++ lines.
	if i := strings.Index(s, " b/"); i >= 0 {
		return strings.TrimPrefix(s[:i], "a/"), s[i+3:]
	}
	return s, s
}

func hunkStart(header string) (int, int) {
	parts := strings.Fields(header)
	parse := func(spec string) int {
		spec = strings.TrimLeft(spec, "-+")
		if i := strings.IndexByte(spec, ','); i >= 0 {
			spec = spec[:i]
		}
		n, _ := strconv.Atoi(spec)
		return n
	}
	if len(parts) < 3 {
		return 1, 1
	}
	return parse(parts[1]), parse(parts[2])
}
