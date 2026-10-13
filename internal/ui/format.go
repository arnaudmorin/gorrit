package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

func pad(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	if w := ansi.StringWidth(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

func padLeft(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	if w := ansi.StringWidth(s); w < width {
		s = strings.Repeat(" ", width-w) + s
	}
	return s
}

// labelAbbrev turns "Code-Review" into "CR", "Verified" into "V".
func labelAbbrev(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, "-") {
		if part != "" {
			b.WriteByte(part[0])
		}
	}
	return strings.ToUpper(b.String())
}

func voteExtremes(label gerrit.LabelInfo) (min, max int) {
	values := label.Votes
	if values == nil {
		for _, a := range label.All {
			values = append(values, a.Value)
		}
	}
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

// onRow adds the selection background to a style when the row is selected,
// since nested styles would otherwise reset it.
func onRow(st lipgloss.Style, selected bool) lipgloss.Style {
	if selected {
		return st.Background(cSelBg)
	}
	return st
}

// voteCell shows the best and worst vote on a label, e.g. "+2-1" for a +2
// alongside a -1, so a veto never hides an approval or the reverse. Votes at
// the label's limit are bold.
func voteCell(label gerrit.LabelInfo, width int, selected bool) string {
	min, max := voteExtremes(label)
	var pos, neg string
	switch {
	case label.Approved != nil:
		pos = onRow(styleGreen.Bold(true), selected).Render(fmt.Sprintf("+%d", max))
	case max > 0:
		pos = onRow(styleGreen, selected).Render(fmt.Sprintf("+%d", max))
	}
	switch {
	case label.Rejected != nil:
		neg = onRow(styleRed.Bold(true), selected).Render(fmt.Sprintf("%d", min))
	case min < 0:
		neg = onRow(styleRed, selected).Render(fmt.Sprintf("%d", min))
	}
	cell := pos + neg
	if cell == "" {
		cell = onRow(styleMuted, selected).Render("·")
	}
	if fill := width - ansi.StringWidth(cell); fill > 0 {
		cell = onRow(lipgloss.NewStyle(), selected).Render(strings.Repeat(" ", fill)) + cell
	}
	return cell
}

func voteValue(v int) string {
	switch {
	case v > 0:
		return styleGreen.Bold(true).Render(fmt.Sprintf("+%d", v))
	case v < 0:
		return styleRed.Bold(true).Render(fmt.Sprintf("%d", v))
	}
	return styleMuted.Render(" 0")
}

func diffStat(ins, del int, selected bool) string {
	return onRow(styleGreen, selected).Render(padLeft(fmt.Sprintf("+%d", ins), 6)) +
		onRow(lipgloss.NewStyle(), selected).Render(" ") +
		onRow(styleRed, selected).Render(pad(fmt.Sprintf("-%d", del), 5))
}

// field renders "label value" with the value wrapped and continuation lines
// indented under the value.
func field(label, value string, labelWidth, width int) string {
	indent := strings.Repeat(" ", 2+labelWidth)
	lines := strings.Split(wrap(value, width-len(indent)), "\n")
	out := "  " + styleMuted.Render(pad(label, labelWidth)) + lines[0]
	for _, l := range lines[1:] {
		out += "\n" + indent + l
	}
	return out
}

func statusBadge(c *gerrit.ChangeInfo) string {
	var parts []string
	switch c.Status {
	case "MERGED":
		parts = append(parts, badgeMerged.Render("MERGED"))
	case "ABANDONED":
		parts = append(parts, badgeAbandon.Render("ABANDONED"))
	default:
		if c.WorkInProgress {
			parts = append(parts, badgeWIP.Render("WIP"))
		} else if c.Submittable {
			parts = append(parts, badgeReady.Render("READY TO SUBMIT"))
		} else {
			parts = append(parts, badgeNew.Render("OPEN"))
		}
	}
	if c.IsPrivate {
		parts = append(parts, badgePrivate.Render("PRIVATE"))
	}
	return strings.Join(parts, " ")
}

// statusCell shows the change state in the list, in the colours used by
// the detail badges.
func statusCell(c *gerrit.ChangeInfo, width int, selected bool) string {
	switch c.Status {
	case "MERGED":
		return onRow(styleGreen, selected).Render(pad("merged", width))
	case "ABANDONED":
		return onRow(styleRed, selected).Render(pad("abandoned", width))
	}
	if c.WorkInProgress {
		return onRow(styleYellow, selected).Render(pad("wip", width))
	}
	return onRow(styleMuted, selected).Render(pad("open", width))
}

func fileStatus(status string) string {
	switch status {
	case "A":
		return styleGreen.Render("A")
	case "D":
		return styleRed.Render("D")
	case "R", "C":
		return styleAccent.Render(status)
	}
	return styleYellow.Render("M")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Wordwrap(s, width, "")
}

func ageText(t time.Time) string {
	a := age(t)
	if a == "now" {
		return "just now"
	}
	return a + " ago"
}

var accountPlaceholder = regexp.MustCompile(`<GERRIT_ACCOUNT_(\d+)>`)

// resolvePlaceholders replaces Gerrit's <GERRIT_ACCOUNT_n> tokens with names
// of accounts known from the change itself.
func resolvePlaceholders(text string, c *gerrit.ChangeInfo) string {
	return accountPlaceholder.ReplaceAllStringFunc(text, func(tok string) string {
		id := accountPlaceholder.FindStringSubmatch(tok)[1]
		if a, ok := knownAccounts(c)[id]; ok {
			return a.Label()
		}
		return "someone"
	})
}

func knownAccounts(c *gerrit.ChangeInfo) map[string]gerrit.AccountInfo {
	m := map[string]gerrit.AccountInfo{fmt.Sprint(c.Owner.AccountID): c.Owner}
	for _, list := range c.Reviewers {
		for _, a := range list {
			m[fmt.Sprint(a.AccountID)] = a
		}
	}
	for id, a := range c.AttentionSet {
		m[id] = a.Account
	}
	for _, msg := range c.Messages {
		if msg.Author != nil {
			m[fmt.Sprint(msg.Author.AccountID)] = *msg.Author
		}
	}
	return m
}
