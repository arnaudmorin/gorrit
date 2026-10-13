package ui

import (
	"context"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

var searchOperators = []string{
	"added", "after", "age", "attention", "author", "before", "branch", "cc", "change", "cherrypickof",
	"comment", "commentby", "commit", "committer", "conflicts", "deleted", "delta", "destination", "directory",
	"extension", "file", "footer", "from", "has", "hashtag", "hasfooter", "inhashtag", "intopic", "is",
	"label", "limit", "mergedafter", "mergedbefore", "message", "onlyextensions", "onlypaths", "owner",
	"ownerin", "parentof", "parentproject", "path", "prefixhashtag", "prefixsubject", "prefixtopic",
	"project", "projects", "query", "ref", "repositories", "repository", "reviewedby", "reviewer",
	"reviewerin", "reviewercount", "revertof", "rule", "size", "status", "subject", "submissionid",
	"topic", "tr", "unresolved", "uploader", "uploaderin", "visibleto",
}

var operatorValues = map[string][]string{
	"is": {"abandoned", "attention", "cc", "cherrypick", "closed", "merge", "mergeable", "merged", "open",
		"owner", "private", "pure", "reviewed", "reviewer", "starred", "submittable", "uploader", "visible",
		"watched", "wip"},
	"has":    {"attention", "draft", "edit", "hashtag", "star", "unresolved"},
	"status": {"abandoned", "closed", "merged", "open"},
}

var accountOperators = map[string]bool{
	"owner": true, "reviewer": true, "cc": true, "author": true, "committer": true, "uploader": true,
	"attention": true, "commentby": true, "reviewedby": true, "from": true, "visibleto": true,
}

var projectOperators = map[string]bool{"project": true, "parentproject": true, "repository": true}

// completer cycles through candidates for the word under the cursor.
type completer struct {
	base       string   // input text before the word being completed
	word       string   // the word as typed when completion started
	candidates []string // full replacements for word
	index      int
	pending    string // word a server lookup is running for
}

type completionsMsg struct {
	base       string
	word       string
	candidates []string
}

func (c *completer) reset() { *c = completer{} }

func (c completer) active() bool { return len(c.candidates) > 0 }

func (c completer) current() string { return c.base + c.candidates[c.index] }

func splitLastWord(input string) (base, word string) {
	i := strings.LastIndexAny(input, " (")
	return input[:i+1], input[i+1:]
}

// complete starts or advances completion for input. It returns the new
// input text and, for server-backed values, a command to fetch candidates.
func (c *completer) complete(client *gerrit.Client, input string) (string, tea.Cmd) {
	if c.active() && input == c.current() {
		c.index = (c.index + 1) % len(c.candidates)
		return c.current(), nil
	}
	c.reset()
	base, word := splitLastWord(input)
	if word == "" {
		return input, nil
	}
	negated := strings.HasPrefix(word, "-")
	plain := strings.TrimPrefix(word, "-")
	op, value, hasOp := strings.Cut(plain, ":")
	prefix := ""
	if negated {
		prefix = "-"
	}
	switch {
	case !hasOp:
		for _, o := range searchOperators {
			if strings.HasPrefix(o, plain) {
				c.candidates = append(c.candidates, prefix+o+":")
			}
		}
		for _, kw := range []string{"AND", "OR", "NOT"} {
			if plain != "" && strings.HasPrefix(kw, strings.ToUpper(plain)) {
				c.candidates = append(c.candidates, kw)
			}
		}
	case operatorValues[op] != nil:
		for _, v := range operatorValues[op] {
			if strings.HasPrefix(v, value) {
				c.candidates = append(c.candidates, prefix+op+":"+v)
			}
		}
	case projectOperators[op] || accountOperators[op]:
		if value == "" || value == "self" {
			return input, nil
		}
		c.pending = word
		c.base = base
		return input, fetchCompletions(client, base, word, prefix+op+":", value, projectOperators[op])
	}
	if len(c.candidates) == 0 {
		c.reset()
		return input, nil
	}
	c.base, c.word = base, word
	return c.current(), nil
}

func fetchCompletions(client *gerrit.Client, base, word, opPrefix, value string, project bool) tea.Cmd {
	return func() tea.Msg {
		var out []string
		if project {
			names, err := client.SuggestProjects(context.Background(), value, 15)
			if err != nil {
				return completionsMsg{base: base, word: word}
			}
			for _, n := range names {
				out = append(out, opPrefix+n)
			}
		} else {
			accounts, err := client.SuggestAccounts(context.Background(), value, 10)
			if err != nil {
				return completionsMsg{base: base, word: word}
			}
			for _, a := range accounts {
				switch {
				case a.Username != "":
					out = append(out, opPrefix+a.Username)
				case a.Email != "":
					out = append(out, opPrefix+a.Email)
				}
			}
			sort.Strings(out)
		}
		return completionsMsg{base: base, word: word, candidates: out}
	}
}

// apply installs fetched candidates if the user has not moved on, and
// returns the completed input.
func (c *completer) apply(msg completionsMsg, input string) (string, bool) {
	if msg.word != c.pending || input != msg.base+msg.word || len(msg.candidates) == 0 {
		c.reset()
		return input, false
	}
	c.base, c.word, c.candidates, c.index, c.pending = msg.base, msg.word, msg.candidates, 0, ""
	return c.current(), true
}

// view lists the candidates with the current one highlighted.
func (c completer) view(width int) string {
	if !c.active() {
		return ""
	}
	var parts []string
	used := 0
	for i, cand := range c.candidates {
		label := strings.TrimPrefix(cand, c.base)
		if i == c.index {
			label = styleTabActive.Render(" " + label + " ")
		} else {
			label = styleMuted.Render(" " + label + " ")
		}
		used += len(cand) + 3
		if used > width-6 {
			parts = append(parts, styleMuted.Render("…"))
			break
		}
		parts = append(parts, label)
	}
	return " " + strings.Join(parts, " ")
}
