package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

// commandOrder mirrors the Gerrit download dialog; unknown commands follow.
var commandOrder = []string{"Checkout", "Cherry Pick", "Cherry-Pick", "Format Patch", "Format-Patch", "Pull", "Branch", "Reset"}

type downloadModel struct {
	open    bool
	schemes []string
	scheme  int
	names   []string
	cursor  int
	fetch   map[string]gerrit.FetchInfo
}

func newDownload(fetch map[string]gerrit.FetchInfo) downloadModel {
	d := downloadModel{open: true, fetch: fetch}
	for scheme := range fetch {
		d.schemes = append(d.schemes, scheme)
	}
	rank := func(s string) int {
		switch s {
		case "ssh":
			return 0
		case "http", "https":
			return 1
		}
		return 2
	}
	sort.SliceStable(d.schemes, func(i, j int) bool {
		if rank(d.schemes[i]) != rank(d.schemes[j]) {
			return rank(d.schemes[i]) < rank(d.schemes[j])
		}
		return d.schemes[i] < d.schemes[j]
	})
	d.refreshNames()
	return d
}

func (d *downloadModel) refreshNames() {
	d.names = d.names[:0]
	if len(d.schemes) == 0 {
		return
	}
	cmds := d.fetch[d.schemes[d.scheme]].Commands
	seen := map[string]bool{}
	for _, n := range commandOrder {
		if _, ok := cmds[n]; ok {
			d.names = append(d.names, n)
			seen[n] = true
		}
	}
	for _, n := range sortedKeys(cmds) {
		if !seen[n] {
			d.names = append(d.names, n)
		}
	}
	d.cursor = min(d.cursor, max(len(d.names)-1, 0))
}

func (d downloadModel) selected() (name, command string) {
	if len(d.schemes) == 0 || len(d.names) == 0 {
		return "", ""
	}
	name = d.names[d.cursor]
	return name, d.fetch[d.schemes[d.scheme]].Commands[name]
}

type downloadDoneMsg struct {
	notice string
}

func (d downloadModel) Update(msg tea.KeyMsg) (downloadModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "D":
		d.open = false
	case "j", "down":
		if d.cursor < len(d.names)-1 {
			d.cursor++
		}
	case "k", "up":
		if d.cursor > 0 {
			d.cursor--
		}
	case "s", "tab", "left", "right", "h", "l":
		if len(d.schemes) > 0 {
			d.scheme = (d.scheme + 1) % len(d.schemes)
			d.refreshNames()
		}
	case "y", "c":
		name, command := d.selected()
		if command == "" {
			return d, nil
		}
		d.open = false
		return d, func() tea.Msg {
			if err := copyToClipboard(command); err != nil {
				return downloadDoneMsg{notice: styleError.Render("copy failed: " + err.Error() + " ")}
			}
			return downloadDoneMsg{notice: styleGreen.Render(name + " command copied ")}
		}
	case "enter":
		name, command := d.selected()
		if command == "" {
			return d, nil
		}
		d.open = false
		script := fmt.Sprintf("echo '$ %s'; %s; echo; read -p 'Press enter to return to gorrit' _", strings.ReplaceAll(command, "'", `'"'"'`), command)
		cmd := exec.Command("sh", "-c", script)
		return d, tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return downloadDoneMsg{notice: styleError.Render(name + " failed: " + err.Error() + " ")}
			}
			return downloadDoneMsg{notice: styleGreen.Render(name + " done ")}
		})
	}
	return d, nil
}

func (d downloadModel) View(width int) string {
	var b strings.Builder
	if len(d.schemes) == 0 {
		b.WriteString(styleMuted.Render("No download commands available for this revision."))
		return styleBox.Width(min(width-2, 100)).Render(b.String())
	}
	var tabs []string
	for i, s := range d.schemes {
		if i == d.scheme {
			tabs = append(tabs, styleTabActive.Render(" "+s+" "))
		} else {
			tabs = append(tabs, styleTab.Render(" "+s+" "))
		}
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Download") + "   " + strings.Join(tabs, " ") + "\n\n")
	cmds := d.fetch[d.schemes[d.scheme]].Commands
	inner := min(width-6, 118)
	for i, n := range d.names {
		marker := "  "
		name := styleMuted.Render(pad(n, 13))
		if i == d.cursor {
			marker = styleAccent.Render("▌ ")
			name = styleAccent.Bold(true).Render(pad(n, 13))
		}
		b.WriteString(marker + name + "\n")
		for _, line := range strings.Split(wrap(cmds[n], inner-4), "\n") {
			b.WriteString("    " + styleText.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + styleMuted.Render("j/k select · s scheme · y copy · enter run here · esc close"))
	return styleBox.Width(inner + 4).Render(strings.TrimRight(b.String(), "\n"))
}
