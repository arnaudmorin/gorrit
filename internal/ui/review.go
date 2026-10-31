package ui

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

type reviewState int

const (
	reviewRunning reviewState = iota
	reviewDone
	reviewFailed
)

type aiReview struct {
	state    reviewState
	patchSet int
	output   string
}

// reviewStore holds the AI reviews of this session and those kept in dir
// from earlier ones. The list and detail views share it.
type reviewStore struct {
	name    string
	command string
	dir     string
	byID    map[string]*aiReview
	frame   int
	ticking bool
}

type reviewRequestMsg struct{ id string }

type reviewDoneMsg struct {
	id       string
	patchSet int
	output   string
	err      error
}

type reviewTickMsg struct{}

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func newReviewStore(name, command, dir string) *reviewStore {
	s := &reviewStore{name: name, command: command, dir: dir, byID: map[string]*aiReview{}}
	s.loadSaved()
	return s
}

func defaultReviewStore() *reviewStore {
	return newReviewStore(config.DefaultReviewName, config.DefaultReviewCommand, "")
}

// ReviewDir is where finished reviews are kept, one directory per server.
func ReviewDir(baseURL string) string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return filepath.Join(cache, "gorrit", "reviews", url.PathEscape(host))
}

func (s *reviewStore) loadSaved() {
	if s.dir == "" {
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".txt")
		if !ok {
			continue
		}
		id, err := url.PathUnescape(name)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		head, body, _ := strings.Cut(string(raw), "\n")
		ps, err := strconv.Atoi(strings.TrimPrefix(head, "patch set "))
		if err != nil {
			continue
		}
		s.byID[id] = &aiReview{state: reviewDone, patchSet: ps, output: body}
	}
}

func (s *reviewStore) save(id string, r *aiReview) error {
	if s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	content := fmt.Sprintf("patch set %d\n%s", r.patchSet, r.output)
	return os.WriteFile(filepath.Join(s.dir, url.PathEscape(id)+".txt"), []byte(content), 0o600)
}

func (s *reviewStore) running() bool {
	for _, r := range s.byID {
		if r.state == reviewRunning {
			return true
		}
	}
	return false
}

// start launches a review of the change unless one is already running.
func (s *reviewStore) start(client *gerrit.Client, id string) tea.Cmd {
	if r := s.byID[id]; r != nil && r.state == reviewRunning {
		return nil
	}
	s.byID[id] = &aiReview{state: reviewRunning}
	cmds := []tea.Cmd{runReview(client, id, s.command)}
	if !s.ticking {
		s.ticking = true
		cmds = append(cmds, reviewTick())
	}
	return tea.Batch(cmds...)
}

func (s *reviewStore) finish(msg reviewDoneMsg) error {
	r := &aiReview{state: reviewDone, patchSet: msg.patchSet, output: strings.TrimSpace(msg.output)}
	if msg.err != nil {
		r.state, r.output = reviewFailed, msg.err.Error()
	}
	s.byID[msg.id] = r
	if r.state == reviewDone {
		return s.save(msg.id, r)
	}
	return nil
}

func reviewTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return reviewTickMsg{} })
}

func runReview(client *gerrit.Client, id, command string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		change, err := client.GetChange(ctx, id)
		if err != nil {
			return reviewDoneMsg{id: id, err: err}
		}
		ps := change.CurrentRevisionNumber
		dir, err := os.MkdirTemp("", "gorrit-review-")
		if err != nil {
			return reviewDoneMsg{id: id, patchSet: ps, err: err}
		}
		defer os.RemoveAll(dir)
		cmd, err := buildDiffCommand(ctx, client, id, command, dir)
		if err != nil {
			return reviewDoneMsg{id: id, patchSet: ps, err: err}
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if msg := lastLine(stderr.String()); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
			return reviewDoneMsg{id: id, patchSet: ps, err: err}
		}
		return reviewDoneMsg{id: id, patchSet: ps, output: stdout.String()}
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	return s[strings.LastIndexByte(s, '\n')+1:]
}

func (s *reviewStore) spinner() string {
	return string(spinnerFrames[s.frame%len(spinnerFrames)])
}

// listTag is the marker shown before the subject of a reviewed change.
func (s *reviewStore) listTag(id string, patchSet int, st func(base lipgloss.Style) lipgloss.Style) string {
	r := s.byID[id]
	if r == nil {
		return ""
	}
	switch r.state {
	case reviewRunning:
		return st(styleYellow).Render(s.spinner() + " " + s.name)
	case reviewFailed:
		return st(styleRed).Render("✗ " + s.name)
	}
	if r.patchSet < patchSet {
		return st(styleMuted).Render(fmt.Sprintf("✓ %s ps%d", s.name, r.patchSet))
	}
	tag := st(styleGreen).Render("✓ " + s.name)
	if v := verdict(r.output); v != "" {
		tag += st(lipgloss.NewStyle()).Render(" ") + st(verdictStyle(v)).Bold(true).Render(v)
	}
	return tag
}

// section renders the review on the summary page.
func (s *reviewStore) section(id string, patchSet, width int) string {
	r := s.byID[id]
	if r == nil {
		return ""
	}
	title := strings.ToUpper(s.name[:1]) + s.name[1:] + " review"
	switch r.state {
	case reviewRunning:
		return rule(title, width) + "\n" + styleYellow.Render("  "+s.spinner()+" "+s.name+" is reviewing the change…") + "\n\n"
	case reviewFailed:
		return rule(title+" · failed", width) + "\n" + wrap(styleRed.Render("  "+r.output), width) + "\n\n"
	}
	title += fmt.Sprintf(" · patch set %d", r.patchSet)
	if r.patchSet < patchSet {
		title += fmt.Sprintf(", now %d · a to review again", patchSet)
	} else if n := len(parseNotes(r.output)); n > 0 {
		title += fmt.Sprintf(" · %d inline in the diff", n)
	}
	if v := verdict(r.output); v != "" {
		title += " · " + verdictStyle(v).Bold(true).Render(v)
	}
	return rule(title, width) + "\n" + quote(r.output, width) + "\n"
}

var verdictLine = regexp.MustCompile(`(?mi)^\W*verdict\W*([+-]1)\b`)

// verdict is the "Verdict: +1" or "-1" the review ends its summary with.
func verdict(output string) string {
	if m := verdictLine.FindStringSubmatch(output); m != nil {
		return m[1]
	}
	return ""
}

func verdictStyle(v string) lipgloss.Style {
	if v == "+1" {
		return styleGreen
	}
	return styleRed
}

// reviewNote is a remark of the review about one line of the new side.
type reviewNote struct {
	path string
	line int
	text string
}

// reviewNotes are the line remarks of a review, labelled with the tool name.
type reviewNotes struct {
	name  string
	items []reviewNote
}

// noteStart matches "path:line: remark", also in list items, in backquotes
// and with extra line numbers or a dash before the remark. The path needs a
// dot or a slash.
var noteStart = regexp.MustCompile("^\\s*(?:[-*]\\s+|\\d+[.)]\\s+)?`?(?:[ab]/)?([\\w+-]*[./][\\w./+-]*):(\\d+)`?[\\d,\\s-]*?(?::|\\s[—–-])\\s*(.*)$")

// parseNotes picks the "path:line: remark" findings out of a review. A
// remark runs until the next finding or a blank line.
func parseNotes(output string) []reviewNote {
	var notes []reviewNote
	open := false
	for _, line := range strings.Split(output, "\n") {
		if m := noteStart.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			notes = append(notes, reviewNote{path: m[1], line: n, text: strings.TrimSpace(m[3])})
			open = true
			continue
		}
		if strings.TrimSpace(line) == "" {
			open = false
			continue
		}
		if open {
			last := &notes[len(notes)-1]
			last.text += "\n" + strings.TrimSpace(line)
		}
	}
	return notes
}

// notes returns the line remarks of the review of id when it covers patchSet.
func (s *reviewStore) notes(id string, patchSet int) reviewNotes {
	out := reviewNotes{name: s.name}
	if r := s.byID[id]; r != nil && r.state == reviewDone && r.patchSet == patchSet {
		out.items = parseNotes(r.output)
	}
	return out
}
