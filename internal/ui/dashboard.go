package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

var dashboardQueryOptions = []string{"LABELS", "DETAILED_ACCOUNTS", "SUBMITTABLE", "STAR"}

const filterLimit = 50

// Tab indexes: 0..n-1 are the configured filters, tabSearch an ad-hoc query.
const tabSearch = -1

// statusFilter narrows every query of the current tab by change status.
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

type section struct {
	title   string
	query   string
	changes []gerrit.ChangeInfo
}

type dashboardLoadedMsg struct {
	sections []section
	err      error
}

type moreLoadedMsg struct {
	section int
	query   string
	changes []gerrit.ChangeInfo
	err     error
}

type openChangeMsg struct{ id string }

type starToggleMsg struct {
	id      string
	starred bool
}

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

type row struct {
	section int
	change  int
}

type dashboardModel struct {
	client      *gerrit.Client
	selfID      int
	filters     []config.Filter
	tab         int
	status      statusFilter
	limit       int
	sections    []section
	rows        []row
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

func newDashboard(client *gerrit.Client, filters []config.Filter) dashboardModel {
	ti := textinput.New()
	ti.Placeholder = "status:open project:foo ..."
	ti.Prompt = "/ "
	if len(filters) == 0 {
		filters = config.AnonymousFilters
		if client.Authenticated() {
			filters = config.DefaultFilters
		}
	}
	m := dashboardModel{
		client:  client,
		filters: filters,
		limit:   filterLimit,
		loading: true,
		search:  ti,
	}
	m.applyTab(0)
	return m
}

// applyTab sets the sections for a filter tab without loading.
func (m *dashboardModel) applyTab(tab int) {
	m.tab = tab
	m.status = statusOpen
	if tab >= 0 && tab < len(m.filters) {
		f := m.filters[tab]
		m.sections = []section{{title: f.Name, query: f.Query}}
	}
}

// showTab switches the dashboard to a tab and reloads it.
func (m *dashboardModel) showTab(tab int) tea.Cmd {
	m.applyTab(tab)
	m.cursor = 0
	m.offset = 0
	m.err = nil
	m.loading = true
	return m.load()
}

func (m dashboardModel) load() tea.Cmd {
	sections, client, limit, status := m.sections, m.client, m.limit, m.status
	return func() tea.Msg {
		queries := make([]string, len(sections))
		for i, s := range sections {
			queries[i] = status.apply(s.query)
		}
		results, err := client.QueryChanges(context.Background(), queries, gerrit.QueryOptions{
			Limit:   limit,
			Options: dashboardQueryOptions,
		})
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		out := make([]section, len(sections))
		for i, s := range sections {
			s.changes = results[i]
			out[i] = s
		}
		return dashboardLoadedMsg{sections: out}
	}
}

// loadMore fetches the next page of one section.
func (m *dashboardModel) loadMore(si int) tea.Cmd {
	if m.loadingMore || si < 0 || si >= len(m.sections) {
		return nil
	}
	sec := m.sections[si]
	if n := len(sec.changes); n == 0 || !sec.changes[n-1].MoreChanges {
		return nil
	}
	m.loadingMore = true
	client, limit, start, query := m.client, m.limit, len(sec.changes), m.status.apply(sec.query)
	return func() tea.Msg {
		results, err := client.QueryChanges(context.Background(), []string{query}, gerrit.QueryOptions{
			Limit:   limit,
			Start:   start,
			Options: dashboardQueryOptions,
		})
		if err != nil {
			return moreLoadedMsg{section: si, query: sec.query, err: err}
		}
		return moreLoadedMsg{section: si, query: sec.query, changes: results[0]}
	}
}

func (m dashboardModel) bodyHeight() int {
	h := m.height - 3
	if m.err != nil {
		h--
	}
	return max(h, 1)
}

// adjustScroll moves the viewport only as far as needed to keep the cursor
// visible, keeping a couple of lines of context above it.
func (m *dashboardModel) adjustScroll() {
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

func (m *dashboardModel) rebuildRows() {
	m.rows = m.rows[:0]
	for si, s := range m.sections {
		for ci := range s.changes {
			m.rows = append(m.rows, row{section: si, change: ci})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m dashboardModel) selected() *gerrit.ChangeInfo {
	if len(m.rows) == 0 {
		return nil
	}
	r := m.rows[m.cursor]
	return &m.sections[r.section].changes[r.change]
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	m, cmd := m.update(msg)
	m.adjustScroll()
	return m, cmd
}

func (m dashboardModel) update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case dashboardLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.sections = msg.sections
			m.rebuildRows()
		}
		return m, nil
	case starDoneMsg:
		if msg.err != nil {
			m.notice = styleError.Render("star: " + msg.err.Error() + " ")
			return m, nil
		}
		for si := range m.sections {
			for ci := range m.sections[si].changes {
				c := &m.sections[si].changes[ci]
				if gerrit.ChangeID(c.Project, c.Number) == msg.id {
					c.Starred = msg.starred
				}
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
	case moreLoadedMsg:
		m.loadingMore = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.section < len(m.sections) && m.sections[msg.section].query == msg.query {
			sec := &m.sections[msg.section]
			if n := len(sec.changes); n > 0 {
				sec.changes[n-1].MoreChanges = false
			}
			sec.changes = append(sec.changes, msg.changes...)
			m.rebuildRows()
		}
		return m, nil
	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.rows)-1 {
				next := m.rows[m.cursor+1]
				if next.section != m.rows[m.cursor].section {
					if cmd := m.loadMore(m.rows[m.cursor].section); cmd != nil {
						return m, cmd
					}
				}
				m.cursor++
			} else if len(m.rows) > 0 {
				return m, m.loadMore(m.rows[m.cursor].section)
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			m.cursor = len(m.rows) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
		case "ctrl+d", "pgdown":
			m.cursor = min(m.cursor+m.pageSize(), max(len(m.rows)-1, 0))
		case "ctrl+u", "pgup":
			m.cursor = max(m.cursor-m.pageSize(), 0)
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
				url := fmt.Sprintf("%s/c/%s/+/%d", m.client.BaseURL(), c.Project, c.Number)
				return m, func() tea.Msg {
					_ = exec.Command("xdg-open", url).Start()
					return nil
				}
			}
		case "r":
			m.loading = true
			m.err = nil
			return m, m.load()
		case "/":
			m.searching = true
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
	}
	return m, nil
}

func (m dashboardModel) updateSearch(msg tea.KeyMsg) (dashboardModel, tea.Cmd) {
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
		q := strings.TrimSpace(m.search.Value())
		m.searching = false
		m.complete.reset()
		m.search.Blur()
		if q == "" {
			return m, nil
		}
		m.tab = tabSearch
		m.sections = []section{{title: "Search: " + q, query: q}}
		m.limit = filterLimit
		m.status = statusAll
		m.cursor = 0
		m.loading = true
		m.err = nil
		return m, m.load()
	}
	m.complete.reset()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

func (m dashboardModel) pageSize() int {
	return max(m.height-4, 1)
}

var dashboardLabels = []string{"Code-Review", "Verified", "Workflow"}

func (m dashboardModel) View() string {
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

func (m *dashboardModel) reload() tea.Cmd {
	m.cursor = 0
	m.offset = 0
	m.err = nil
	m.loading = true
	return m.load()
}

// tabsView renders the dashboard and filter tabs with the active one highlighted.
func (m dashboardModel) tabsView() string {
	tab := func(index int, name string) string {
		label := fmt.Sprintf(" %d %s ", index, name)
		if index-1 == m.tab {
			return styleTabActive.Render(label)
		}
		return styleTab.Render(label)
	}
	var parts []string
	for i, f := range m.filters {
		if i >= 9 {
			break
		}
		parts = append(parts, tab(i+1, f.Name))
	}
	if m.tab == tabSearch {
		parts = append(parts, styleTabActive.Render(" / "+ansi.Truncate(m.sections[0].query, 40, "…")+" "))
	}
	left := " " + strings.Join(parts, " ")
	right := styleMuted.Render("status ") + styleAccent.Bold(true).Render(m.status.String()) + styleMuted.Render(" (s) ")
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m dashboardModel) renderRows() ([]string, int) {
	labels := dashboardLabels
	const (
		wNum    = 8
		wFlags  = 2
		wOwner  = 18
		wProj   = 26
		wBranch = 14
		wStat   = 12
		wAge    = 4
		gutter  = 2
	)
	const wVote = 4
	labelWidth := len(labels) * (wVote + 1)
	wSubject := m.width - gutter - wNum - wFlags - wOwner - wProj - wBranch - labelWidth - wStat - wAge - 8
	if wSubject < 20 {
		wSubject = 20
	}

	header := strings.Repeat(" ", gutter) + padLeft("#", wNum) + " " + pad("", wFlags) + " " + pad("SUBJECT", wSubject) + " " +
		pad("OWNER", wOwner) + " " + pad("PROJECT", wProj) + " " + pad("BRANCH", wBranch) + " "
	for _, l := range labels {
		header += padLeft(labelAbbrev(l), wVote) + " "
	}
	header += pad("SIZE", wStat) + " " + padLeft("AGE", wAge)

	var lines []string
	cursorLine := 0
	rowIdx := 0
	for si, s := range m.sections {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, rule(fmt.Sprintf("%s (%d)", s.title, len(s.changes)), m.width))
		if len(s.changes) == 0 {
			lines = append(lines, styleMuted.Render("  nothing here"))
			continue
		}
		lines = append(lines, styleColHead.Render(header))
		for i := range s.changes {
			c := &s.changes[i]
			selected := rowIdx == m.cursor
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
			subject := st(styleText).Render(pad(c.Subject, wSubject))
			prefix := ""
			if c.WorkInProgress {
				prefix += st(styleYellow).Render("WIP ")
			}
			if c.IsPrivate {
				prefix += st(styleSection).Render("PRIVATE ")
			}
			if c.Topic != "" {
				prefix += st(styleAccent).Render(c.Topic) + st(styleMuted).Render(" · ")
			}
			if prefix != "" {
				rest := wSubject - ansi.StringWidth(prefix)
				if rest < 10 {
					prefix = ansi.Truncate(prefix, wSubject-10, "…")
					rest = wSubject - ansi.StringWidth(prefix)
				}
				subject = prefix + st(styleText).Render(pad(c.Subject, rest))
			}
			owner := st(styleOwner).Render(pad(c.Owner.Label(), wOwner))
			project := st(styleMuted).Render(pad(c.Project, wProj))
			branch := st(styleMuted).Render(pad(c.Branch, wBranch))

			line := number + sp + flag + sp + subject + sp + owner + sp + project + sp + branch + sp
			for _, l := range labels {
				if info, ok := c.Labels[l]; ok {
					line += voteCell(info, wVote, selected) + sp
				} else {
					line += st(lipgloss.NewStyle()).Render(strings.Repeat(" ", wVote+1))
				}
			}
			line += diffStat(c.Insertions, c.Deletions, selected) + sp + st(styleMuted).Render(padLeft(age(c.Updated.Time), wAge))

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
			rowIdx++
		}
		if n := len(s.changes); n > 0 && s.changes[n-1].MoreChanges {
			lines = append(lines, styleMuted.Render("  … more changes match this query, move down past the last row to load them"))
		}
	}
	return lines, cursorLine
}
