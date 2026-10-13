package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

var detailOptions = []string{"CURRENT_REVISION", "CURRENT_COMMIT", "CURRENT_FILES", "DOWNLOAD_COMMANDS", "SUBMIT_REQUIREMENTS", "SUBMITTABLE", "STAR"}

type detailLoadedMsg struct {
	id       string
	change   *gerrit.ChangeInfo
	comments map[string][]gerrit.CommentInfo
	err      error
}

type patchLoadedMsg struct {
	id    string
	files []diffFile
	err   error
}

type replySentMsg struct {
	id  string
	err error
}

type closeDetailMsg struct{}

type page int

const (
	pageSummary page = iota
	pageDiff
)

type detailModel struct {
	client   *gerrit.Client
	bugURL   string
	bots     []string
	selfID   int
	id       string
	change   *gerrit.ChangeInfo
	threads  []thread
	threadAt map[int]int // thread index → content line
	selected int         // selected thread, -1 for none
	loading  bool
	err      error
	notice   string
	viewport viewport.Model
	width    int
	height   int

	page        page
	files       []diffFile
	hasPatch    bool
	diffPos     int // index into diffLines
	diffLines   []int
	diffAnchors map[int]lineAnchor

	showAutomated bool
	download      downloadModel
	vote          voteModel

	composing   bool
	composeKind composeKind
	replyTo     thread     // composeReply
	commentAt   lineAnchor // composeComment
	unresolved  bool
	sending     bool
	compose     textarea.Model
}

func newDetail(client *gerrit.Client, bugURL string, bots []string) detailModel {
	ta := textarea.New()
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
	m.composing = false
	m.viewport.GotoTop()
	return m.load()
}

func (m *detailModel) load() tea.Cmd {
	m.loading = true
	m.err = nil
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

func (m *detailModel) loadPatch() tea.Cmd {
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

// review posts a review on the change and reports back with replySentMsg.
func (m detailModel) review(rev string, input gerrit.ReviewInput) tea.Cmd {
	client, id := m.client, m.id
	return func() tea.Msg {
		err := client.SetReview(context.Background(), id, rev, input)
		return replySentMsg{id: id, err: err}
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
	if pos < 0 {
		pos = 0
	} else {
		pos = min(max(pos+step, 0), len(visible)-1)
	}
	m.selected = visible[pos]
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

func (m *detailModel) showPage(p page) {
	m.page = p
	m.viewport.GotoTop()
	m.diffPos = 0
	m.refreshContent()
	m.scrollToSelected()
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

// moveLineBy moves the diff cursor to the commentable line nearest to the
// current one plus delta content lines, used for paging.
func (m *detailModel) moveLineBy(delta int) {
	if len(m.diffLines) == 0 {
		return
	}
	target := m.diffLines[m.diffPos] + delta
	pos := m.diffPos
	if delta > 0 {
		for pos < len(m.diffLines)-1 && m.diffLines[pos] < target {
			pos++
		}
	} else {
		for pos > 0 && m.diffLines[pos] > target {
			pos--
		}
	}
	m.moveLine(pos - m.diffPos)
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

func (m detailModel) selectedThread() *thread {
	if m.selected < 0 || m.selected >= len(m.threads) {
		return nil
	}
	return &m.threads[m.selected]
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
		m.closeComposer()
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
		return m, m.review(gerrit.RevisionCurrent, gerrit.ReviewInput{Labels: msg.labels, Message: msg.message})
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m detailModel) updateKey(msg tea.KeyMsg) (detailModel, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.composing:
		return m.updateCompose(msg)
	case m.download.open:
		m.download, cmd = m.download.Update(msg)
		return m, cmd
	case m.vote.open:
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
		if m.change != nil {
			if rev := m.change.CurrentRevisionInfo(); rev != nil {
				m.download = newDownload(rev.Fetch)
			}
		}
		return m, nil
	case "esc", "backspace", "h", "q":
		if m.page == pageDiff {
			m.showPage(pageSummary)
			return m, nil
		}
		return m, func() tea.Msg { return closeDetailMsg{} }
	case "enter":
		if m.page == pageSummary && m.change != nil {
			if m.hasPatch {
				m.showPage(pageDiff)
				return m, nil
			}
			return m, m.loadPatch()
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
	case "pgdown", "ctrl+f":
		if m.page == pageDiff {
			m.moveLineBy(m.viewport.Height)
			return m, nil
		}
	case "pgup", "ctrl+b":
		if m.page == pageDiff {
			m.moveLineBy(-m.viewport.Height)
			return m, nil
		}
	case "ctrl+d":
		if m.page == pageDiff {
			m.moveLineBy(m.viewport.Height / 2)
			return m, nil
		}
	case "ctrl+u":
		if m.page == pageDiff {
			m.moveLineBy(-m.viewport.Height / 2)
			return m, nil
		}
	case "c":
		if t := m.selectedThread(); t != nil {
			return m.startReply("", t.unresolved())
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
			return m, openBrowser(bugURL(m.bugURL, refs[0].id))
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
		return m, m.load()
	case "o":
		if m.change != nil {
			return m, openBrowser(m.client.ChangeURL(m.change.Project, m.change.Number))
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
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
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
		b.WriteString(lipgloss.Place(m.width, m.viewport.Height, lipgloss.Center, lipgloss.Center, m.download.View(m.width)))
	case m.vote.open:
		b.WriteString(lipgloss.Place(m.width, m.viewport.Height, lipgloss.Center, lipgloss.Center, m.vote.View(m.width)))
	default:
		b.WriteString(m.viewport.View())
	}
	b.WriteString("\n")
	if m.composing {
		b.WriteString(m.composeView() + "\n")
		if m.composeKind == composeMessage {
			b.WriteString(hints(m.width, keyHint{"ctrl+s", "send"}, keyHint{"esc", "cancel"}))
		} else {
			b.WriteString(hints(m.width, keyHint{"ctrl+s", "send"}, keyHint{"ctrl+u", "toggle resolved"}, keyHint{"esc", "cancel"}))
		}
		return b.String()
	}
	var keys string
	if m.page == pageDiff {
		items := []keyHint{{"j/k", "line"}}
		if m.selectedThread() != nil {
			items = append(items, keyHint{"c", "reply"}, keyHint{"x", "done"})
		} else {
			items = append(items, keyHint{"c", "comment"})
		}
		if len(m.visibleThreads()) > 0 {
			items = append(items, keyHint{"n/N", "threads"})
		}
		items = append(items, keyHint{"d", "external diff"}, keyHint{"esc", "summary"})
		keys = hints(0, items...)
	} else {
		items := []keyHint{{"j/k", "scroll"}, {"enter", "diff"}}
		if len(m.threads) > 0 {
			items = append(items, keyHint{"n/N", "threads"})
		}
		if m.selectedThread() != nil {
			items = append(items, keyHint{"c", "reply"}, keyHint{"x", "done"})
		}
		items = append(items,
			keyHint{"m", "message"}, keyHint{"v", "vote"}, keyHint{"*", "star"}, keyHint{"t", "bots"},
			keyHint{"d", "external diff"}, keyHint{"D", "download"}, keyHint{"o", "browser"}, keyHint{"b", "bug"},
			keyHint{"r", "refresh"}, keyHint{"esc", "back"})
		keys = hints(0, items...)
	}
	b.WriteString(bar(keys, styleBarMuted.Render(fmt.Sprintf("%3.0f%% ", m.viewport.ScrollPercent()*100)), m.width))
	return b.String()
}
