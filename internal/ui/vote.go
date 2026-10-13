package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

type voteLabel struct {
	name    string
	values  []int
	current int
	chosen  int
}

type voteModel struct {
	open    bool
	labels  []voteLabel
	cursor  int
	message textinput.Model
	typing  bool
}

// newVote lists the labels the user may vote on, preselecting their
// current vote.
func newVote(c *gerrit.ChangeInfo, selfID int) voteModel {
	ti := textinput.New()
	ti.Placeholder = "optional message"
	ti.Prompt = ""
	v := voteModel{open: true, message: ti}
	for _, name := range sortedKeys(c.PermittedLabels) {
		var values []int
		for _, raw := range c.PermittedLabels[name] {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err == nil {
				values = append(values, n)
			}
		}
		if len(values) == 0 {
			continue
		}
		sort.Ints(values)
		l := voteLabel{name: name, values: values}
		for _, a := range c.Labels[name].All {
			if a.AccountID == selfID {
				l.current = a.Value
			}
		}
		l.chosen = l.current
		v.labels = append(v.labels, l)
	}
	return v
}

func (v *voteModel) adjust(step int) {
	l := &v.labels[v.cursor]
	idx := 0
	for i, val := range l.values {
		if val == l.chosen {
			idx = i
		}
	}
	idx = min(max(idx+step, 0), len(l.values)-1)
	l.chosen = l.values[idx]
}

// changes returns the votes that differ from the current ones.
func (v voteModel) changes() map[string]int {
	out := map[string]int{}
	for _, l := range v.labels {
		if l.chosen != l.current {
			out[l.name] = l.chosen
		}
	}
	return out
}

type voteSubmitMsg struct {
	labels  map[string]int
	message string
}

func (v voteModel) Update(msg tea.KeyMsg) (voteModel, tea.Cmd) {
	if v.typing {
		switch msg.String() {
		case "esc":
			v.typing = false
			v.message.Blur()
			return v, nil
		case "enter":
			return v.submit()
		}
		var cmd tea.Cmd
		v.message, cmd = v.message.Update(msg)
		return v, cmd
	}
	switch msg.String() {
	case "esc", "q", "v":
		v.open = false
	case "j", "down":
		if v.cursor < len(v.labels)-1 {
			v.cursor++
		}
	case "k", "up":
		if v.cursor > 0 {
			v.cursor--
		}
	case "l", "right", "+", "=":
		if len(v.labels) > 0 {
			v.adjust(1)
		}
	case "h", "left", "-":
		if len(v.labels) > 0 {
			v.adjust(-1)
		}
	case "0":
		if len(v.labels) > 0 {
			v.labels[v.cursor].chosen = 0
		}
	case "i", "m", "tab":
		v.typing = true
		v.message.Focus()
		return v, textinput.Blink
	case "enter":
		return v.submit()
	}
	return v, nil
}

func (v voteModel) submit() (voteModel, tea.Cmd) {
	labels := v.changes()
	message := strings.TrimSpace(v.message.Value())
	if len(labels) == 0 && message == "" {
		return v, nil
	}
	v.open = false
	return v, func() tea.Msg { return voteSubmitMsg{labels: labels, message: message} }
}

func (v voteModel) View(width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Vote") + "\n\n")
	if len(v.labels) == 0 {
		b.WriteString(styleMuted.Render("You cannot vote on this change.") + "\n")
	}
	for i, l := range v.labels {
		marker := "  "
		name := styleMuted.Render(pad(l.name, 18))
		if i == v.cursor && !v.typing {
			marker = styleAccent.Render("▌ ")
			name = styleAccent.Bold(true).Render(pad(l.name, 18))
		}
		var cells []string
		for _, val := range l.values {
			text := fmt.Sprintf("%+d", val)
			if val == 0 {
				text = " 0"
			}
			st := styleMuted
			switch {
			case val > 0:
				st = styleGreen
			case val < 0:
				st = styleRed
			}
			if val == l.chosen {
				cells = append(cells, st.Bold(true).Reverse(true).Render(" "+text+" "))
			} else {
				cells = append(cells, st.Render(" "+text+" "))
			}
		}
		line := marker + name + strings.Join(cells, " ")
		if l.chosen != l.current {
			line += styleYellow.Render(fmt.Sprintf("   was %+d", l.current))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	label := styleMuted.Render("message  ")
	if v.typing {
		label = styleAccent.Bold(true).Render("message  ")
	}
	b.WriteString(label + v.message.View() + "\n\n")
	b.WriteString(styleMuted.Render("j/k label · h/l value · 0 clear · i message · enter submit · esc close"))
	inner := min(width-6, 100)
	return styleBox.Width(inner + 4).Render(b.String())
}
