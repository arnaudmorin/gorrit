package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	cAccent  = lipgloss.AdaptiveColor{Light: "27", Dark: "75"}
	cSection = lipgloss.AdaptiveColor{Light: "97", Dark: "141"}
	cMuted   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	cText    = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	cGreen   = lipgloss.AdaptiveColor{Light: "28", Dark: "78"}
	cRed     = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	cYellow  = lipgloss.AdaptiveColor{Light: "136", Dark: "221"}
	cCyan    = lipgloss.AdaptiveColor{Light: "30", Dark: "80"}
	cBarBg   = lipgloss.AdaptiveColor{Light: "254", Dark: "236"}
	cSelBg   = lipgloss.AdaptiveColor{Light: "189", Dark: "238"}
	cOnColor = lipgloss.AdaptiveColor{Light: "255", Dark: "232"}
	cBot     = lipgloss.AdaptiveColor{Light: "130", Dark: "180"}

	styleLogo      = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cAccent).Padding(0, 1)
	styleBar       = lipgloss.NewStyle().Background(cBarBg).Foreground(cText)
	styleBarMuted  = lipgloss.NewStyle().Background(cBarBg).Foreground(cMuted)
	styleKey       = lipgloss.NewStyle().Background(cBarBg).Foreground(cAccent).Bold(true)
	styleSection   = lipgloss.NewStyle().Bold(true).Foreground(cSection)
	styleColHead   = lipgloss.NewStyle().Foreground(cMuted).Bold(true)
	styleSelected  = lipgloss.NewStyle().Background(cSelBg).Bold(true)
	styleMuted     = lipgloss.NewStyle().Foreground(cMuted)
	styleText      = lipgloss.NewStyle().Foreground(cText)
	styleAccent    = lipgloss.NewStyle().Foreground(cAccent)
	styleOwner     = lipgloss.NewStyle().Foreground(cCyan)
	styleBot       = lipgloss.NewStyle().Foreground(cBot)
	styleGreen     = lipgloss.NewStyle().Foreground(cGreen)
	styleRed       = lipgloss.NewStyle().Foreground(cRed)
	styleYellow    = lipgloss.NewStyle().Foreground(cYellow)
	styleError     = lipgloss.NewStyle().Bold(true).Foreground(cRed)
	styleTab       = lipgloss.NewStyle().Foreground(cMuted)
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cSection)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cMuted).Padding(0, 1)

	badgeBase    = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Padding(0, 1)
	badgeWIP     = badgeBase.Background(cYellow)
	badgePrivate = badgeBase.Background(cSection)
	badgeMerged  = badgeBase.Background(cGreen)
	badgeAbandon = badgeBase.Background(cMuted)
	badgeReady   = badgeBase.Background(cCyan)
	badgeNew     = badgeBase.Background(cAccent)
)

// bar renders a full-width line with left and right content on the bar background.
func bar(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		right = ""
		gap = max(width-ansi.StringWidth(left), 0)
	}
	return left + styleBar.Render(strings.Repeat(" ", gap)) + right
}

type keyHint struct{ key, desc string }

func hints(width int, items ...keyHint) string {
	var b strings.Builder
	b.WriteString(styleBar.Render(" "))
	for i, h := range items {
		if i > 0 {
			b.WriteString(styleBarMuted.Render("  "))
		}
		b.WriteString(styleKey.Render(h.key) + styleBarMuted.Render(" "+h.desc))
	}
	return bar(b.String(), "", width)
}

func rule(title string, width int) string {
	line := styleSection.Render(title) + " "
	if rest := width - ansi.StringWidth(line); rest > 0 {
		line += styleMuted.Render(strings.Repeat("─", rest))
	}
	return line
}
