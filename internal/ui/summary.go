package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

type renderOpts struct {
	selected      int
	showAutomated bool
	bugURL        string
	bots          []string
}

// renderChange renders the summary page and returns the content line of
// each thread header, keyed by thread index.
func renderChange(c *gerrit.ChangeInfo, ts []thread, width int, opts renderOpts) (string, map[int]int) {
	var b strings.Builder
	b.WriteString(renderHeader(c, width, opts.bugURL) + "\n\n")

	if len(c.Labels) > 0 {
		b.WriteString(rule("Labels", width) + "\n")
		for _, name := range sortedKeys(c.Labels) {
			var votes []string
			for _, a := range c.Labels[name].All {
				if a.Value != 0 {
					votes = append(votes, voteValue(a.Value)+" "+a.Label())
				}
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
			switch sr.Status {
			case "SATISFIED", "OVERRIDDEN", "FORCED":
				items = append(items, styleGreen.Render("✔ "+sr.Name))
			case "NOT_APPLICABLE":
				items = append(items, styleMuted.Render("– "+sr.Name))
			default:
				items = append(items, styleRed.Render("✘ "+sr.Name))
			}
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
		b.WriteString(quote(rev.Commit.Message, width) + "\n")
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

// renderHeader draws the boxed summary at the top of the page.
func renderHeader(c *gerrit.ChangeInfo, width int, bugTemplate string) string {
	inner := max(width-4, 20)
	var b strings.Builder
	b.WriteString(wrap(styleAccent.Bold(true).Render(fmt.Sprintf("#%d", c.Number))+"  "+lipgloss.NewStyle().Bold(true).Render(c.Subject), inner) + "\n")
	badges := statusBadge(c)
	if c.Starred {
		badges += "  " + styleYellow.Render("★ starred")
	}
	b.WriteString(badges + "\n")
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
	b.WriteString(wrap(strings.Join(meta, styleMuted.Render("  ·  ")), inner))
	if len(c.Hashtags) > 0 {
		b.WriteString("\n" + styleMuted.Render("hashtags ") + strings.Join(c.Hashtags, ", "))
	}
	for _, ref := range changeBugs(c) {
		b.WriteString("\n" + styleMuted.Render(pad(strings.ToLower(ref.kind)+" bug", 13)) + "#" + ref.id + "  " + styleAccent.Render(bugURL(bugTemplate, ref.id)))
	}
	return styleBox.Width(inner + 2).Render(b.String())
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
		b.WriteString(quote(text, width) + "\n")
	}
	return b.String()
}
