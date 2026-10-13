package ui

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

var detailOptions = []string{"CURRENT_REVISION", "CURRENT_COMMIT", "CURRENT_FILES", "DOWNLOAD_COMMANDS", "SUBMIT_REQUIREMENTS", "SUBMITTABLE", "STAR"}

const composeHeight = 5

type detailLoadedMsg struct {
	id       string
	change   *gerrit.ChangeInfo
	comments map[string][]gerrit.CommentInfo
	err      error
}

type replySentMsg struct {
	id  string
	err error
}

type patchLoadedMsg struct {
	id    string
	files []diffFile
	err   error
}

type closeDetailMsg struct{}

type page int

const (
	pageSummary page = iota
	pageDiff
)

type detailModel struct {
	client      *gerrit.Client
	bugURL      string
	bots        []string
	selfID      int
	id          string
	change      *gerrit.ChangeInfo
	threads     []thread
	threadAt    map[int]int
	selected    int
	page        page
	files       []diffFile
	hasPatch    bool
	diffPos     int // index into diffLines
	diffLines   []int
	diffAnchors map[int]lineAnchor
	newComment  *lineAnchor
	loading     bool
	err         error
	notice      string
	viewport    viewport.Model
	width       int
	height      int

	showAutomated bool
	download      downloadModel
	vote          voteModel
	composing     bool
	messageMode   bool
	sending       bool
	unresolved    bool
	compose       textarea.Model
}

func newDetail(client *gerrit.Client, bugURL string, bots []string) detailModel {
	ta := textarea.New()
	ta.Placeholder = "Write your reply…"
	ta.Prompt = "│ "
	ta.ShowLineNumbers = false
	ta.SetHeight(composeHeight - 2)
	ta.CharLimit = 0
	return detailModel{client: client, bugURL: bugURL, bots: bots, viewport: viewport.New(0, 0), selected: -1, compose: ta}
}

func (m *detailModel) open(id string) tea.Cmd {
	m.id = id
	m.change = nil
	m.threads = nil
	m.selected = -1
	m.page = pageSummary
	m.files = nil
	m.hasPatch = false
	m.err = nil
	m.loading = true
	m.composing = false
	m.viewport.GotoTop()
	return m.load()
}

func (m detailModel) load() tea.Cmd {
	client, id := m.client, m.id
	return func() tea.Msg {
		change, err := client.GetChangeDetail(context.Background(), id, detailOptions...)
		if err != nil {
			return detailLoadedMsg{id: id, err: err}
		}
		comments, err := client.ListComments(context.Background(), id)
		return detailLoadedMsg{id: id, change: change, comments: comments, err: err}
	}
}

func (m *detailModel) resize(width, height int) {
	m.width = width
	m.height = height
	m.compose.SetWidth(width - 2)
	m.layout()
	m.refreshContent()
}

func (m *detailModel) layout() {
	m.viewport.Width = m.width
	h := m.height - 2
	if m.composing {
		h -= composeHeight
	}
	m.viewport.Height = max(h, 1)
}

func (m *detailModel) refreshContent() {
	if m.change == nil {
		return
	}
	var content string
	switch m.page {
	case pageDiff:
		cursor := -1
		if m.diffPos >= 0 && m.diffPos < len(m.diffLines) {
			cursor = m.diffLines[m.diffPos]
		}
		r := renderDiff(m.files, m.threads, m.change, m.width, m.selected, m.change.CurrentRevisionNumber, cursor)
		content, m.threadAt, m.diffAnchors, m.diffLines = r.content, r.threadAt, r.anchors, r.lines
		if m.diffPos >= len(m.diffLines) {
			m.diffPos = max(len(m.diffLines)-1, 0)
		}
	default:
		content, m.threadAt = renderChange(m.change, m.threads, m.width, renderOpts{selected: m.selected, showAutomated: m.showAutomated, bugURL: m.bugURL, bots: m.bots})
	}
	m.viewport.SetContent(content)
}

// visibleThreads lists the thread indexes reachable on the current page.
func (m detailModel) visibleThreads() []int {
	var out []int
	for i := range m.threads {
		if m.page == pageDiff && m.threads[i].root().PatchSet != m.change.CurrentRevisionNumber {
			continue
		}
		out = append(out, i)
	}
	return out
}

// moveThread selects the next (+1) or previous (-1) visible thread.
func (m *detailModel) moveThread(step int) {
	visible := m.visibleThreads()
	if len(visible) == 0 {
		return
	}
	pos := -1
	for i, idx := range visible {
		if idx == m.selected {
			pos = i
		}
	}
	switch {
	case pos < 0:
		pos = 0
	default:
		pos = min(max(pos+step, 0), len(visible)-1)
	}
	m.selectThread(visible[pos])
}

func (m *detailModel) selectThread(i int) {
	m.selected = i
	m.refreshContent()
	m.scrollToSelected()
}

func (m *detailModel) scrollToSelected() {
	line, ok := m.threadAt[m.selected]
	if !ok {
		return
	}
	if line < m.viewport.YOffset || line >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(max(line-2, 0))
	}
}

func (m *detailModel) openDiffPage() tea.Cmd {
	if m.change == nil {
		return nil
	}
	if !m.hasPatch {
		m.loading = true
		client, id := m.client, m.id
		return func() tea.Msg {
			raw, err := client.GetPatch(context.Background(), id, gerrit.RevisionCurrent)
			if err != nil {
				return patchLoadedMsg{id: id, err: err}
			}
			return patchLoadedMsg{id: id, files: parsePatch(raw)}
		}
	}
	m.showPage(pageDiff)
	return nil
}

func (m *detailModel) showPage(p page) {
	m.page = p
	m.viewport.GotoTop()
	m.diffPos = 0
	m.refreshContent()
	if _, visible := m.threadAt[m.selected]; visible {
		m.scrollToSelected()
	}
}

// moveLine moves the diff cursor and drops any thread selection so that
// "c" comments on the line rather than replying.
func (m *detailModel) moveLine(step int) {
	if len(m.diffLines) == 0 {
		return
	}
	m.diffPos = min(max(m.diffPos+step, 0), len(m.diffLines)-1)
	m.selected = -1
	m.refreshContent()
	line := m.diffLines[m.diffPos]
	switch {
	case line < m.viewport.YOffset+2:
		m.viewport.SetYOffset(max(line-2, 0))
	case line >= m.viewport.YOffset+m.viewport.Height-1:
		m.viewport.SetYOffset(line - m.viewport.Height + 2)
	}
}

func (m detailModel) cursorAnchor() *lineAnchor {
	if m.page != pageDiff || m.diffPos < 0 || m.diffPos >= len(m.diffLines) {
		return nil
	}
	a, ok := m.diffAnchors[m.diffLines[m.diffPos]]
	if !ok {
		return nil
	}
	return &a
}

func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case detailLoadedMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.change = msg.change
			m.threads = threads(msg.comments)
			if m.selected >= len(m.threads) {
				m.selected = len(m.threads) - 1
			}
			m.refreshContent()
		}
		return m, nil
	case patchLoadedMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.notice = styleError.Render("patch: " + msg.err.Error() + " ")
			return m, nil
		}
		m.files = msg.files
		m.hasPatch = true
		m.showPage(pageDiff)
		return m, nil
	case replySentMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.sending = false
		if msg.err != nil {
			m.notice = styleError.Render("post failed: " + msg.err.Error() + " ")
			return m, nil
		}
		m.notice = styleGreen.Render("posted ")
		m.composing = false
		m.compose.Reset()
		m.layout()
		m.loading = true
		m.hasPatch = false
		return m, m.load()
	case downloadDoneMsg:
		m.notice = msg.notice
		return m, nil
	case starDoneMsg:
		if msg.err != nil {
			m.notice = styleError.Render("star: " + msg.err.Error() + " ")
		} else if m.change != nil && msg.id == m.id {
			m.change.Starred = msg.starred
			m.refreshContent()
		}
		return m, nil
	case voteSubmitMsg:
		m.sending = true
		m.notice = styleYellow.Render("posting vote… ")
		client, id := m.client, m.id
		input := gerrit.ReviewInput{Labels: msg.labels, Message: msg.message}
		return m, func() tea.Msg {
			_, err := client.SetReview(context.Background(), id, gerrit.RevisionCurrent, input)
			return replySentMsg{id: id, err: err}
		}
	case tea.KeyMsg:
		if m.composing {
			return m.updateCompose(msg)
		}
		if m.download.open {
			var cmd tea.Cmd
			m.download, cmd = m.download.Update(msg)
			return m, cmd
		}
		if m.vote.open {
			var cmd tea.Cmd
			m.vote, cmd = m.vote.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "v":
			if m.change == nil {
				return m, nil
			}
			if !m.client.Authenticated() {
				m.notice = styleError.Render("voting needs a username and token ")
				return m, nil
			}
			m.vote = newVote(m.change, m.selfID)
			m.vote.message.Width = min(m.width-20, 80)
			return m, nil
		case "D":
			if rev := m.currentRevision(); rev != nil {
				m.download = newDownload(rev.Fetch)
			}
			return m, nil
		case "esc", "backspace", "h", "q":
			if m.page == pageDiff {
				m.showPage(pageSummary)
				return m, nil
			}
			return m, func() tea.Msg { return closeDetailMsg{} }
		case "enter":
			if m.page == pageSummary {
				return m, m.openDiffPage()
			}
			return m, nil
		case "n":
			m.moveThread(1)
			return m, nil
		case "N":
			m.moveThread(-1)
			return m, nil
		case "j", "down":
			if m.page == pageDiff {
				m.moveLine(1)
				return m, nil
			}
		case "k", "up":
			if m.page == pageDiff {
				m.moveLine(-1)
				return m, nil
			}
		case "c":
			if m.selected >= 0 && m.selected < len(m.threads) {
				return m.startReply("", m.threads[m.selected].unresolved())
			}
			if a := m.cursorAnchor(); a != nil {
				return m.startNewComment(*a)
			}
		case "x":
			return m.startReply("Done", false)
		case "m":
			return m.startMessage()
		case "*":
			if m.change != nil && m.client.Authenticated() {
				return m, toggleStar(m.client, m.id, !m.change.Starred)
			}
		case "b":
			if refs := changeBugs(m.change); len(refs) > 0 {
				url := bugURL(m.bugURL, refs[0].id)
				return m, func() tea.Msg {
					_ = exec.Command("xdg-open", url).Start()
					return nil
				}
			}
			m.notice = styleMuted.Render("no bug referenced ")
			return m, nil
		case "t":
			m.showAutomated = !m.showAutomated
			m.refreshContent()
			return m, nil
		case "d":
			id := m.id
			return m, func() tea.Msg { return openDiffMsg{id: id} }
		case "r":
			m.loading = true
			m.err = nil
			return m, m.load()
		case "o":
			if m.change != nil {
				url := fmt.Sprintf("%s/c/%s/+/%d", m.client.BaseURL(), m.change.Project, m.change.Number)
				return m, func() tea.Msg {
					_ = exec.Command("xdg-open", url).Start()
					return nil
				}
			}
		case "g":
			m.viewport.GotoTop()
			if m.page == pageDiff {
				m.moveLine(-len(m.diffLines))
			}
		case "G":
			m.viewport.GotoBottom()
			if m.page == pageDiff {
				m.moveLine(len(m.diffLines))
			}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// startReply opens the composer on the selected thread with an initial
// message and resolution state.
func (m detailModel) startReply(text string, unresolved bool) (detailModel, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.threads) {
		return m, nil
	}
	m.messageMode = false
	m.newComment = nil
	m.unresolved = unresolved
	return m.openComposer(text)
}

// startNewComment opens the composer for a fresh inline comment on a line.
func (m detailModel) startNewComment(a lineAnchor) (detailModel, tea.Cmd) {
	m.messageMode = false
	m.newComment = &a
	m.unresolved = true
	return m.openComposer("")
}

// startMessage opens the composer for a change-level message.
func (m detailModel) startMessage() (detailModel, tea.Cmd) {
	if m.change == nil {
		return m, nil
	}
	m.messageMode = true
	m.newComment = nil
	return m.openComposer("")
}

func (m detailModel) openComposer(text string) (detailModel, tea.Cmd) {
	if !m.client.Authenticated() {
		m.notice = styleError.Render("posting needs a username and token ")
		return m, nil
	}
	m.composing = true
	m.compose.SetValue(text)
	m.compose.Placeholder = "Write your reply…"
	if m.messageMode {
		m.compose.Placeholder = "Write a message on the change…"
	}
	if m.newComment != nil {
		m.compose.Placeholder = "Write your comment…"
	}
	m.layout()
	m.compose.Focus()
	return m, textarea.Blink
}

func (m detailModel) updateCompose(msg tea.KeyMsg) (detailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.composing = false
		m.compose.Blur()
		m.layout()
		return m, nil
	case "ctrl+u":
		if !m.messageMode {
			m.unresolved = !m.unresolved
		}
		return m, nil
	case "ctrl+s":
		text := strings.TrimSpace(m.compose.Value())
		if text == "" || m.sending {
			return m, nil
		}
		m.sending = true
		if m.messageMode {
			m.notice = styleYellow.Render("posting message… ")
			return m, m.sendMessage(text)
		}
		if m.newComment != nil {
			m.notice = styleYellow.Render("posting comment… ")
			return m, m.sendNewComment(*m.newComment, text, m.unresolved)
		}
		m.notice = styleYellow.Render("posting reply… ")
		return m, m.sendReply(m.threads[m.selected], text, m.unresolved)
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(msg)
	return m, cmd
}

func (m detailModel) sendMessage(text string) tea.Cmd {
	client, id := m.client, m.id
	return func() tea.Msg {
		_, err := client.SetReview(context.Background(), id, gerrit.RevisionCurrent, gerrit.ReviewInput{Message: text})
		return replySentMsg{id: id, err: err}
	}
}

func (m detailModel) sendNewComment(a lineAnchor, text string, unresolved bool) tea.Cmd {
	client, id := m.client, m.id
	rev := strconv.Itoa(m.change.CurrentRevisionNumber)
	input := gerrit.ReviewInput{Comments: map[string][]gerrit.CommentInput{a.path: {{
		Line: a.line, Side: a.side, Message: text, Unresolved: &unresolved,
	}}}}
	return func() tea.Msg {
		_, err := client.SetReview(context.Background(), id, rev, input)
		return replySentMsg{id: id, err: err}
	}
}

func (m detailModel) sendReply(t thread, text string, unresolved bool) tea.Cmd {
	client, id := m.client, m.id
	rev := strconv.Itoa(t.root().PatchSet)
	input := gerrit.ReviewInput{Comments: map[string][]gerrit.CommentInput{t.path: {t.replyInput(text, unresolved)}}}
	return func() tea.Msg {
		_, err := client.SetReview(context.Background(), id, rev, input)
		return replySentMsg{id: id, err: err}
	}
}

func (m detailModel) View() string {
	var b strings.Builder
	title := "change " + m.id
	if m.change != nil {
		title = fmt.Sprintf("%s · change %d", m.change.Project, m.change.Number)
		if m.page == pageDiff {
			title += fmt.Sprintf(" · diff of patch set %d", m.change.CurrentRevisionNumber)
		}
	}
	left := styleLogo.Render("gorrit") + styleBar.Render(" ") + styleBarMuted.Render(title)
	right := m.notice
	if m.loading {
		right = styleBar.Foreground(cYellow).Render("loading… ")
	}
	b.WriteString(bar(left, right, m.width) + "\n")
	switch {
	case m.err != nil:
		b.WriteString(styleError.Render(" ✘ " + m.err.Error()))
		b.WriteString(strings.Repeat("\n", max(m.viewport.Height-1, 0)))
	case m.download.open:
		panel := m.download.View(m.width)
		b.WriteString(lipgloss.Place(m.width, m.viewport.Height, lipgloss.Center, lipgloss.Center, panel))
	case m.vote.open:
		panel := m.vote.View(m.width)
		b.WriteString(lipgloss.Place(m.width, m.viewport.Height, lipgloss.Center, lipgloss.Center, panel))
	default:
		b.WriteString(m.viewport.View())
	}
	b.WriteString("\n")
	if m.composing {
		b.WriteString(m.composeView() + "\n")
		if m.messageMode {
			b.WriteString(hints(m.width, keyHint{"ctrl+s", "send"}, keyHint{"esc", "cancel"}))
		} else {
			b.WriteString(hints(m.width,
				keyHint{"ctrl+s", "send"}, keyHint{"ctrl+u", "toggle resolved"}, keyHint{"esc", "cancel"}))
		}
		return b.String()
	}
	var keys string
	if m.page == pageDiff {
		keys = hints(0,
			keyHint{"j/k", "line"}, keyHint{"c", "comment"}, keyHint{"n/N", "threads"}, keyHint{"x", "done"},
			keyHint{"d", "external diff"}, keyHint{"esc", "summary"})
	} else {
		keys = hints(0,
			keyHint{"j/k", "scroll"}, keyHint{"enter", "diff"}, keyHint{"n/N", "threads"}, keyHint{"c", "reply"}, keyHint{"x", "done"}, keyHint{"m", "message"}, keyHint{"v", "vote"}, keyHint{"*", "star"},
			keyHint{"t", "bots"}, keyHint{"d", "external diff"}, keyHint{"D", "download"}, keyHint{"o", "browser"}, keyHint{"b", "bug"}, keyHint{"r", "refresh"}, keyHint{"esc", "back"})
	}
	b.WriteString(bar(keys, styleBarMuted.Render(fmt.Sprintf("%3.0f%% ", m.viewport.ScrollPercent()*100)), m.width))
	return b.String()
}

func changeBugs(c *gerrit.ChangeInfo) []bugRef {
	if c == nil {
		return nil
	}
	if rev := c.CurrentRevisionInfo(); rev != nil && rev.Commit != nil {
		return parseBugRefs(rev.Commit.Message)
	}
	return nil
}

func (m detailModel) currentRevision() *gerrit.RevisionInfo {
	if m.change == nil {
		return nil
	}
	return m.change.CurrentRevisionInfo()
}

func (m detailModel) composeView() string {
	if m.messageMode {
		return rule(fmt.Sprintf("Message on change %d", m.change.Number), m.width) + "\n" + m.compose.View()
	}
	if m.newComment != nil {
		state := styleGreen.Render("resolved")
		if m.unresolved {
			state = styleRed.Render("unresolved")
		}
		return rule("New comment on "+m.newComment.String(), m.width-14) + "  " + state + "\n" + m.compose.View()
	}
	t := m.threads[m.selected]
	state := styleGreen.Render("resolved")
	if m.unresolved {
		state = styleRed.Render("unresolved")
	}
	header := rule(fmt.Sprintf("Reply on %s %s", t.path, commentLocation(t.root())), m.width-14) + "  " + state
	return header + "\n" + m.compose.View()
}

type renderOpts struct {
	selected      int
	showAutomated bool
	bugURL        string
	bots          []string
}

func renderChange(c *gerrit.ChangeInfo, ts []thread, width int, opts renderOpts) (string, map[int]int) {
	var b strings.Builder
	inner := max(width-4, 20)

	var head strings.Builder
	head.WriteString(wrap(styleAccent.Bold(true).Render(fmt.Sprintf("#%d", c.Number))+"  "+lipgloss.NewStyle().Bold(true).Render(c.Subject), inner) + "\n")
	badges := statusBadge(c)
	if c.Starred {
		badges += "  " + styleYellow.Render("★ starred")
	}
	head.WriteString(badges + "\n")
	meta := []string{
		styleMuted.Render("owner ") + styleOwner.Render(c.Owner.Label()),
		styleMuted.Render("project ") + c.Project,
		styleMuted.Render("branch ") + c.Branch,
	}
	if c.Topic != "" {
		meta = append(meta, styleMuted.Render("topic ")+styleAccent.Render(c.Topic))
	}
	meta = append(meta, styleMuted.Render("updated ")+ageText(c.Updated.Time))
	if c.Status == "MERGED" && c.Submitter != nil {
		meta = append(meta, styleMuted.Render("submitted by ")+c.Submitter.Label()+" "+ageText(c.Submitted.Time))
	}
	head.WriteString(wrap(strings.Join(meta, styleMuted.Render("  ·  ")), inner))
	if len(c.Hashtags) > 0 {
		head.WriteString("\n" + styleMuted.Render("hashtags ") + strings.Join(c.Hashtags, ", "))
	}
	for _, ref := range changeBugs(c) {
		head.WriteString("\n" + styleMuted.Render(pad(strings.ToLower(ref.kind)+" bug", 13)) + "#" + ref.id + "  " + styleAccent.Render(bugURL(opts.bugURL, ref.id)))
	}
	b.WriteString(styleBox.Width(inner+2).Render(head.String()) + "\n\n")

	if len(c.Labels) > 0 {
		b.WriteString(rule("Labels", width) + "\n")
		for _, name := range sortedKeys(c.Labels) {
			label := c.Labels[name]
			var votes []string
			for _, a := range label.All {
				if a.Value == 0 {
					continue
				}
				votes = append(votes, voteValue(a.Value)+" "+a.Label())
			}
			line := "  " + pad(name, 18) + " "
			if len(votes) == 0 {
				line += styleMuted.Render("no votes")
			} else {
				line += strings.Join(votes, styleMuted.Render("  "))
			}
			b.WriteString(wrap(line, width) + "\n")
		}
		b.WriteString("\n")
	}

	if len(c.SubmitRequirements) > 0 {
		b.WriteString(rule("Submit requirements", width) + "\n")
		var items []string
		for _, sr := range c.SubmitRequirements {
			var item string
			switch sr.Status {
			case "SATISFIED", "OVERRIDDEN", "FORCED":
				item = styleGreen.Render("✔ " + sr.Name)
			case "NOT_APPLICABLE":
				item = styleMuted.Render("– " + sr.Name)
			default:
				item = styleRed.Render("✘ " + sr.Name)
			}
			items = append(items, item)
		}
		b.WriteString(wrap("  "+strings.Join(items, "   "), width) + "\n\n")
	}

	b.WriteString(rule("People", width) + "\n")
	if reviewers := c.Reviewers["REVIEWER"]; len(reviewers) > 0 {
		b.WriteString(field("reviewers", joinAccounts(reviewers), 11, width) + "\n")
	}
	if ccs := c.Reviewers["CC"]; len(ccs) > 0 {
		b.WriteString(field("cc", joinAccounts(ccs), 11, width) + "\n")
	}
	if len(c.AttentionSet) > 0 {
		var who []string
		for _, key := range sortedKeys(c.AttentionSet) {
			a := c.AttentionSet[key]
			who = append(who, styleYellow.Render("● "+a.Account.Label())+styleMuted.Render(" ("+resolvePlaceholders(a.Reason, c)+")"))
		}
		b.WriteString(field("attention", strings.Join(who, ", "), 11, width) + "\n")
	}
	b.WriteString("\n")

	rev := c.CurrentRevisionInfo()
	if rev != nil && rev.Commit != nil {
		short := c.CurrentRevision
		if len(short) > 10 {
			short = short[:10]
		}
		b.WriteString(rule(fmt.Sprintf("Commit message · patch set %d · %s · %s", rev.Number, short, rev.Uploader.Label()), width) + "\n")
		b.WriteString(quote(rev.Commit.Message, width))
		b.WriteString("\n")
	}

	if rev != nil && len(rev.Files) > 0 {
		paths := sortedKeys(rev.Files)
		b.WriteString(rule(fmt.Sprintf("Files (%d) · ", len(paths))+diffStat(c.Insertions, c.Deletions, false), width) + "\n")
		for _, p := range paths {
			if p == "/COMMIT_MSG" || p == "/MERGE_LIST" {
				continue
			}
			f := rev.Files[p]
			name := p
			if f.OldPath != "" {
				name = styleMuted.Render(f.OldPath+" → ") + p
			}
			line := "  " + fileStatus(f.Status) + " " + pad(name, max(width-18, 10)) + " "
			if f.Binary {
				line += styleMuted.Render("binary")
			} else {
				line += diffStat(f.LinesInserted, f.LinesDeleted, false)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	var positions map[int]int
	if section, pos := renderComments(ts, width, c, opts.selected, strings.Count(b.String(), "\n")); section != "" {
		b.WriteString(section + "\n")
		positions = pos
	}

	humans, bots := splitMessages(c.Messages, opts.bots)
	if len(humans) > 0 {
		b.WriteString(renderMessages(humans, fmt.Sprintf("Messages (%d)", len(humans)), c, width, false))
	}
	switch {
	case len(bots) == 0:
	case opts.showAutomated:
		b.WriteString(renderMessages(bots, fmt.Sprintf("Automated messages (%d) · t to hide", len(bots)), c, width, true))
	default:
		b.WriteString(rule(fmt.Sprintf("Automated messages (%d) · hidden, t to show", len(bots)), width) + "\n\n")
	}
	return b.String(), positions
}

// quote renders text with a muted left bar, wrapped to width.
func quote(text string, width int) string {
	prefix := styleMuted.Render("  │ ")
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		for _, wrapped := range strings.Split(wrap(line, width-4), "\n") {
			b.WriteString(prefix + wrapped + "\n")
		}
	}
	return b.String()
}

func joinAccounts(accts []gerrit.AccountInfo) string {
	names := make([]string, len(accts))
	for i, a := range accts {
		names[i] = a.Label()
	}
	return strings.Join(names, ", ")
}

// isAutomated recognises Gerrit-tagged messages, configured bot accounts,
// and authors named like "Foo CI".
func isAutomated(msg gerrit.ChangeMessageInfo, botNames []string) bool {
	if strings.HasPrefix(msg.Tag, "autogenerated:") {
		return true
	}
	if msg.Author == nil {
		return false
	}
	name, user := strings.ToLower(msg.Author.Name), strings.ToLower(msg.Author.Username)
	if strings.HasSuffix(name, " ci") || strings.HasSuffix(user, "-ci") || strings.HasSuffix(user, "_ci") {
		return true
	}
	for _, b := range botNames {
		b = strings.ToLower(b)
		if b == name || b == user {
			return true
		}
	}
	return false
}

// splitMessages separates human reviews from CI and Gerrit system messages,
// each newest first.
func splitMessages(msgs []gerrit.ChangeMessageInfo, botNames []string) (humans, bots []gerrit.ChangeMessageInfo) {
	for _, m := range msgs {
		if isAutomated(m, botNames) {
			bots = append(bots, m)
		} else {
			humans = append(humans, m)
		}
	}
	newestFirst := func(list []gerrit.ChangeMessageInfo) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Date.After(list[j].Date.Time) })
	}
	newestFirst(humans)
	newestFirst(bots)
	return humans, bots
}

var jobResult = regexp.MustCompile(`\b(SUCCESS|FAILURE|POST_FAILURE|RETRY_LIMIT|TIMED_OUT|NODE_FAILURE|ERROR|SKIPPED|ABORTED)\b`)

func colorJobResults(line string) string {
	return jobResult.ReplaceAllStringFunc(line, func(word string) string {
		switch word {
		case "SUCCESS":
			return styleGreen.Render(word)
		case "SKIPPED", "ABORTED":
			return styleMuted.Render(word)
		}
		return styleRed.Bold(true).Render(word)
	})
}

func renderMessages(msgs []gerrit.ChangeMessageInfo, title string, c *gerrit.ChangeInfo, width int, automated bool) string {
	var b strings.Builder
	b.WriteString(rule(title, width) + "\n")
	for _, msg := range msgs {
		author := "Gerrit"
		if msg.Author != nil {
			author = msg.Author.Label()
		}
		name := styleOwner.Bold(true).Render(author)
		if automated {
			name = styleBot.Bold(true).Render(author)
		}
		hdr := name + styleMuted.Render(fmt.Sprintf("  %s · %s", msg.Date.Local().Format("2006-01-02 15:04"), ageText(msg.Date.Time)))
		if msg.RevisionNumber > 0 {
			hdr += styleMuted.Render(fmt.Sprintf(" · PS%d", msg.RevisionNumber))
		}
		if msg.Tag != "" {
			hdr += styleMuted.Render(" · " + strings.TrimPrefix(msg.Tag, "autogenerated:"))
		}
		b.WriteString(hdr + "\n")
		text := resolvePlaceholders(strings.TrimSpace(msg.Message), c)
		if automated {
			text = colorJobResults(text)
		}
		b.WriteString(quote(text, width))
		b.WriteString("\n")
	}
	return b.String()
}
