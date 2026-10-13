package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/config"
	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

type view int

const (
	viewDashboard view = iota
	viewDetail
)

type selfLoadedMsg struct {
	account *gerrit.AccountInfo
	err     error
}

type Model struct {
	client      *gerrit.Client
	diffCommand string
	view        view
	dashboard   dashboardModel
	detail      detailModel
}

type Options struct {
	DiffCommand string
	Filters     []config.Filter
	BugURL      string
	Bots        []string
}

func New(client *gerrit.Client, opts Options) Model {
	return Model{
		client:      client,
		diffCommand: opts.DiffCommand,
		dashboard:   newDashboard(client, opts.Filters),
		detail:      newDetail(client, opts.BugURL, opts.Bots),
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.dashboard.refresh()}
	if m.client.Authenticated() {
		client := m.client
		cmds = append(cmds, func() tea.Msg {
			acct, err := client.GetSelf(context.Background())
			return selfLoadedMsg{account: acct, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// Update handles app-wide messages itself, delivers load results to the view
// that asked for them whether or not it is showing, and sends everything
// else (keys, mouse, resize) to the visible view.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.dashboard.width, m.dashboard.height = msg.Width, msg.Height
		m.detail.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.view == viewDashboard && !m.dashboard.searching {
				return m, tea.Quit
			}
		}
	case selfLoadedMsg:
		if msg.err == nil && msg.account != nil {
			m.dashboard.selfID = msg.account.AccountID
			m.detail.selfID = msg.account.AccountID
		}
		return m, nil
	case openChangeMsg:
		m.view = viewDetail
		return m, m.detail.open(msg.id)
	case closeDetailMsg:
		m.view = viewDashboard
		return m, nil
	case starDoneMsg:
		m.dashboard, cmd = m.dashboard.Update(msg)
		m.detail, _ = m.detail.Update(msg)
		return m, cmd
	case openDiffMsg:
		m.setNotice(styleYellow.Render("preparing diff… "))
		return m, prepareDiff(m.client, msg.id, m.diffCommand)
	case diffReadyMsg:
		if msg.err != nil {
			m.setNotice(styleError.Render("diff: " + msg.err.Error() + " "))
			return m, nil
		}
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			msg.cleanup()
			return diffDoneMsg{err: err}
		})
	case diffDoneMsg:
		m.setNotice("")
		var exit *exec.ExitError
		// git diff exits 1 when files differ, which is the normal case.
		if errors.As(msg.err, &exit) && exit.ExitCode() != 1 {
			m.setNotice(styleError.Render(fmt.Sprintf("diff viewer exited with %d ", exit.ExitCode())))
		}
		return m, nil
	case dashboardLoadedMsg, moreLoadedMsg, completionsMsg:
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	case detailLoadedMsg, patchLoadedMsg, replySentMsg, downloadDoneMsg, voteSubmitMsg:
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	switch m.view {
	case viewDashboard:
		m.dashboard, cmd = m.dashboard.Update(msg)
	case viewDetail:
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

func (m *Model) setNotice(s string) {
	m.dashboard.notice = s
	m.detail.notice = s
}

func (m Model) View() string {
	if m.view == viewDetail {
		return m.detail.View()
	}
	return m.dashboard.View()
}

func openBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("xdg-open", url).Start()
		return nil
	}
}
