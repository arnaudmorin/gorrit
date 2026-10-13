package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

var listQueryOptions = []string{"LABELS", "DETAILED_ACCOUNTS", "SUBMITTABLE", "STAR"}

var listLabels = []string{"Code-Review", "Verified", "Workflow"}

const pageLimit = 50

// Tab indexes: 0..n-1 are the configured filters, tabSearch an ad-hoc query.
const tabSearch = -1

// statusFilter narrows the query of the current tab by change status.
type statusFilter int

const (
	statusAll statusFilter = iota
	statusOpen
	statusMerged
)

func (f statusFilter) String() string {
	switch f {
	case statusOpen:
		return "open"
	case statusMerged:
		return "merged"
	}
	return "all"
}

// next cycles open → merged → all.
func (f statusFilter) next() statusFilter {
	switch f {
	case statusOpen:
		return statusMerged
	case statusMerged:
		return statusAll
	}
	return statusOpen
}

func (f statusFilter) apply(query string) string {
	switch f {
	case statusOpen:
		return "(" + query + ") status:open"
	case statusMerged:
		return "(" + query + ") status:merged"
	}
	return query
}

// listLoadedMsg carries the first page of a query, moreLoadedMsg the
// following ones. Both name their query so stale answers can be ignored.
type listLoadedMsg struct {
	query   string
	changes []gerrit.ChangeInfo
	err     error
}

type moreLoadedMsg struct {
	query   string
	changes []gerrit.ChangeInfo
	err     error
}

type openChangeMsg struct{ id string }

type starDoneMsg struct {
	id      string
	starred bool
	err     error
}

func toggleStar(client *gerrit.Client, id string, starred bool) tea.Cmd {
	return func() tea.Msg {
		err := client.SetStarred(context.Background(), id, starred)
		return starDoneMsg{id: id, starred: starred, err: err}
	}
}

type listModel struct {
	client      *gerrit.Client
	selfID      int
	filters     []config.Filter
	tab         int
	status      statusFilter
	title       string
	query       string
	changes     []gerrit.ChangeInfo
	cursor      int
	offset      int
	loading     bool
	loadingMore bool
	err         error
	notice      string
	width       int
	height      int

	searching bool
	search    textinput.Model
	complete  completer
}

func newList(client *gerrit.Client, filters []config.Filter) listModel {
	ti := textinput.New()
	ti.Placeholder = "status:open project:foo ..."
	ti.Prompt = "/ "
	if len(filters) == 0 {
		filters = config.AnonymousFilters
		if client.Authenticated() {
			filters = config.DefaultFilters
		}
	}
	m := listModel{client: client, filters: filters, loading: true, search: ti}
	m.applyTab(0)
	return m
}

// applyTab points the list at a filter tab without loading it.
func (m *listModel) applyTab(tab int) {
	m.tab = tab
	m.status = statusOpen
	m.title = m.filters[tab].Name
	m.query = m.filters[tab].Query
}

func (m *listModel) showTab(tab int) tea.Cmd {
	m.applyTab(tab)
	return m.reload()
}

// reload fetches the current query from the top.
func (m *listModel) reload() tea.Cmd {
	m.cursor = 0
	m.offset = 0
	return m.refresh()
}

// refresh fetches the current query again, keeping the cursor where it is.
func (m *listModel) refresh() tea.Cmd {
	m.err = nil
	m.loading = true
	client, query := m.client, m.fullQuery()
	return func() tea.Msg {
		changes, err := client.QueryChanges(context.Background(), query, gerrit.QueryOptions{
			Limit:   pageLimit,
			Options: listQueryOptions,
		})
		return listLoadedMsg{query: query, changes: changes, err: err}
	}
}

func (m listModel) fullQuery() string {
	return m.status.apply(m.query)
}

func (m listModel) hasMore() bool {
	n := len(m.changes)
	return n > 0 && m.changes[n-1].MoreChanges
}

// loadMore fetches the next page when Gerrit said there is one.
func (m *listModel) loadMore() tea.Cmd {
	if m.loadingMore || !m.hasMore() {
		return nil
	}
	m.loadingMore = true
	client, query, start := m.client, m.fullQuery(), len(m.changes)
	return func() tea.Msg {
		changes, err := client.QueryChanges(context.Background(), query, gerrit.QueryOptions{
			Limit:   pageLimit,
			Start:   start,
			Options: listQueryOptions,
		})
		return moreLoadedMsg{query: query, changes: changes, err: err}
	}
}

func (m listModel) bodyHeight() int {
	h := m.height - 3
	if m.err != nil {
		h--
	}
	return max(h, 1)
}

// adjustScroll moves the viewport only as far as needed to keep the cursor
// visible, keeping a couple of lines of context above it.
func (m *listModel) adjustScroll() {
	lines, cursorLine := m.renderRows()
	h := m.bodyHeight()
	switch {
	case cursorLine < m.offset+2:
		m.offset = max(cursorLine-2, 0)
	case cursorLine >= m.offset+h:
		m.offset = cursorLine - h + 1
	}
	m.offset = min(m.offset, max(len(lines)-h, 0))
}

func (m *listModel) clampCursor() {
	m.cursor = min(max(m.cursor, 0), max(len(m.changes)-1, 0))
}

func (m listModel) selected() *gerrit.ChangeInfo {
	if len(m.changes) == 0 {
		return nil
	}
	return &m.changes[m.cursor]
}

func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	m, cmd := m.update(msg)
	m.adjustScroll()
	return m, cmd
}

func (m listModel) update(msg tea.Msg) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case listLoadedMsg:
		if msg.query != m.fullQuery() {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.changes = msg.changes
			m.clampCursor()
		}
		return m, nil
	case moreLoadedMsg:
		m.loadingMore = false
		if msg.query != m.fullQuery() {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if n := len(m.changes); n > 0 {
			m.changes[n-1].MoreChanges = false
		}
		m.changes = append(m.changes, msg.changes...)
		return m, nil
	case starDoneMsg:
		if msg.err != nil {
			m.notice = styleError.Render("star: " + msg.err.Error() + " ")
			return m, nil
		}
		for i := range m.changes {
			c := &m.changes[i]
			if gerrit.ChangeID(c.Project, c.Number) == msg.id {
				c.Starred = msg.starred
			}
		}
		return m, nil
	case completionsMsg:
		if m.searching {
			if text, ok := m.complete.apply(msg, m.search.Value()); ok {
				m.search.SetValue(text)
				m.search.CursorEnd()
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateKey(msg)
	}
	return m, nil
}

func (m listModel) updateKey(msg tea.KeyMsg) (listModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor >= len(m.changes)-1 {
			return m, m.loadMore()
		}
		m.cursor++
	case "k", "up":
		m.cursor--
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.changes) - 1
	case "ctrl+d", "pgdown":
		m.cursor += m.bodyHeight()
	case "ctrl+u", "pgup":
		m.cursor -= m.bodyHeight()
	case "enter", "l":
		if c := m.selected(); c != nil {
			id := gerrit.ChangeID(c.Project, c.Number)
			return m, func() tea.Msg { return openChangeMsg{id: id} }
		}
	case "d":
		if c := m.selected(); c != nil {
			id := gerrit.ChangeID(c.Project, c.Number)
			return m, func() tea.Msg { return openDiffMsg{id: id} }
		}
	case "*":
		if c := m.selected(); c != nil && m.client.Authenticated() {
			return m, toggleStar(m.client, gerrit.ChangeID(c.Project, c.Number), !c.Starred)
		}
	case "o":
		if c := m.selected(); c != nil {
			return m, openBrowser(m.client.ChangeURL(c.Project, c.Number))
		}
	case "r":
		return m, m.refresh()
	case "/":
		m.searching = true
		m.search.SetValue(strings.TrimSpace(m.query) + " ")
		m.search.CursorEnd()
		m.search.Focus()
		return m, textinput.Blink
	case "esc":
		if m.tab == tabSearch {
			return m, m.showTab(0)
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		tab := int(msg.String()[0]-'0') - 1
		if tab < len(m.filters) && tab != m.tab {
			return m, m.showTab(tab)
		}
	case "s":
		m.status = m.status.next()
		return m, m.reload()
	case "tab", "shift+tab":
		count := min(len(m.filters), 9)
		cur := max(m.tab, 0)
		if msg.String() == "tab" {
			cur = (cur + 1) % count
		} else {
			cur = (cur + count - 1) % count
		}
		return m, m.showTab(cur)
	}
	m.clampCursor()
	return m, nil
}

func (m listModel) updateSearch(msg tea.KeyMsg) (listModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.complete.active() {
			m.complete.reset()
			return m, nil
		}
		m.searching = false
		m.search.Blur()
		return m, nil
	case "tab":
		text, cmd := m.complete.complete(m.client, m.search.Value())
		m.search.SetValue(text)
		m.search.CursorEnd()
		return m, cmd
	case "enter":
		q := rewriteBareTerms(strings.TrimSpace(m.search.Value()))
		m.searching = false
		m.complete.reset()
		m.search.Blur()
		if q == "" {
			return m, nil
		}
		m.tab = tabSearch
		m.status = statusAll
		m.title = "Search: " + q
		m.query = q
		return m, m.reload()
	}
	m.complete.reset()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	var b strings.Builder
	left := styleLogo.Render("gorrit") + styleBar.Render(" ") + styleBarMuted.Render(m.client.BaseURL())
	right := m.notice
	if m.loading || m.loadingMore {
		right = styleBar.Foreground(cYellow).Render("loading… ")
	}
	b.WriteString(bar(left, right, m.width) + "\n")
	b.WriteString(m.tabsView() + "\n")

	if m.err != nil {
		b.WriteString(styleError.Render(" ✘ "+m.err.Error()) + "\n")
	}

	bodyHeight := m.bodyHeight()
	if m.searching && m.complete.active() {
		bodyHeight = max(bodyHeight-1, 1)
	}
	lines, _ := m.renderRows()
	offset := min(m.offset, len(lines))
	end := min(offset+bodyHeight, len(lines))
	b.WriteString(strings.Join(lines[offset:end], "\n"))
	for i := end - offset; i < bodyHeight; i++ {
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if m.searching {
		if m.complete.active() {
			b.WriteString(m.complete.view(m.width) + "\n")
		}
		b.WriteString(bar(styleBar.Render(" ")+m.search.View(), styleBarMuted.Render("tab completes "), m.width))
	} else {
		b.WriteString(hints(m.width,
			keyHint{"j/k", "move"}, keyHint{"enter", "open"}, keyHint{"tab/1-9", "tabs"}, keyHint{"s", "status"}, keyHint{"d", "external diff"},
			keyHint{"*", "star"}, keyHint{"o", "browser"}, keyHint{"/", "search"}, keyHint{"r", "refresh"}, keyHint{"esc", "clear search"}, keyHint{"q", "quit"}))
	}
	return b.String()
}

// tabsView renders the filter tabs with the active one highlighted.
func (m listModel) tabsView() string {
	var parts []string
	for i, f := range m.filters[:min(len(m.filters), 9)] {
		label := fmt.Sprintf(" %d %s ", i+1, f.Name)
		if i == m.tab {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTab.Render(label))
		}
	}
	if m.tab == tabSearch {
		parts = append(parts, styleTabActive.Render(" / "+ansi.Truncate(m.query, 40, "…")+" "))
	}
	left := " " + strings.Join(parts, " ")
	right := styleMuted.Render("status ") + styleAccent.Bold(true).Render(m.status.String()) + styleMuted.Render(" (s) ")
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderRows renders the change table and reports the line holding the cursor.
func (m listModel) renderRows() ([]string, int) {
	const (
		wNum    = 8
		wFlags  = 2
		wStatus = 9
		wOwner  = 18
		wProj   = 26
		wBranch = 14
		wVote   = 4
		wMine   = 13
		wStat   = 12
		wAge    = 4
		gutter  = 2
	)
	labelWidth := len(listLabels) * (wVote + 1)
	wSubject := max(m.width-gutter-wNum-wFlags-wStatus-wOwner-wProj-wBranch-labelWidth-wMine-wStat-wAge-10, 20)

	lines := []string{rule(fmt.Sprintf("%s (%d)", m.title, len(m.changes)), m.width)}
	if len(m.changes) == 0 {
		lines = append(lines, styleMuted.Render("  nothing here"))
		return lines, 0
	}

	header := strings.Repeat(" ", gutter) + padLeft("#", wNum) + " " + pad("", wFlags) + " " + pad("STATUS", wStatus) + " " + pad("SUBJECT", wSubject) + " " +
		pad("OWNER", wOwner) + " " + pad("PROJECT", wProj) + " " + pad("BRANCH", wBranch) + " "
	for _, l := range listLabels {
		header += padLeft(labelAbbrev(l), wVote) + " "
	}
	header += pad("MINE", wMine) + " " + pad("SIZE", wStat) + " " + padLeft("AGE", wAge)
	lines = append(lines, styleColHead.Render(header))

	cursorLine := 0
	for i := range m.changes {
		c := &m.changes[i]
		selected := i == m.cursor
		st := func(base lipgloss.Style) lipgloss.Style { return onRow(base, selected) }
		sp := st(lipgloss.NewStyle()).Render(" ")

		flag := sp
		if _, ok := c.AttentionSet[fmt.Sprint(m.selfID)]; ok && m.selfID != 0 {
			flag = st(styleYellow).Render("●")
		}
		if c.Starred {
			flag += st(styleYellow).Render("★")
		} else {
			flag += sp
		}
		number := st(styleMuted).Render(padLeft(fmt.Sprint(c.Number), wNum))
		prefix := ""
		if c.IsPrivate {
			prefix += st(styleSection).Render("PRIVATE ")
		}
		if c.Topic != "" {
			prefix += st(styleAccent).Render(c.Topic) + st(styleMuted).Render(" · ")
		}
		if ansi.StringWidth(prefix) > wSubject-10 {
			prefix = ansi.Truncate(prefix, wSubject-10, "…")
		}
		subject := prefix + st(styleText).Render(pad(c.Subject, wSubject-ansi.StringWidth(prefix)))
		owner := st(styleOwner).Render(pad(c.Owner.Label(), wOwner))
		project := st(styleMuted).Render(pad(c.Project, wProj))
		branch := st(styleMuted).Render(pad(c.Branch, wBranch))

		line := number + sp + flag + sp + statusCell(c, wStatus, selected) + sp + subject + sp + owner + sp + project + sp + branch + sp
		for _, l := range listLabels {
			if info, ok := c.Labels[l]; ok {
				line += voteCell(info, wVote, selected) + sp
			} else {
				line += st(lipgloss.NewStyle()).Render(strings.Repeat(" ", wVote+1))
			}
		}
		mine := myVotes(c, m.selfID, selected)
		if mine == "" {
			mine = st(styleMuted).Render("·")
		}
		if fill := wMine - ansi.StringWidth(mine); fill > 0 {
			mine += st(lipgloss.NewStyle()).Render(strings.Repeat(" ", fill))
		}
		line += mine + sp + diffStat(c.Insertions, c.Deletions, selected) + sp + st(styleMuted).Render(padLeft(age(c.Updated.Time), wAge))

		if selected {
			cursorLine = len(lines)
			line = st(styleAccent).Render("▌ ") + line
			if fill := m.width - ansi.StringWidth(line); fill > 0 {
				line += st(lipgloss.NewStyle()).Render(strings.Repeat(" ", fill))
			}
		} else {
			line = strings.Repeat(" ", gutter) + line
		}
		lines = append(lines, line)
	}
	if m.hasMore() {
		lines = append(lines, styleMuted.Render("  … more changes match this query, move down past the last row to load them"))
	}
	return lines, cursorLine
}
