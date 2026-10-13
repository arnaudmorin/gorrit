package ui

import "strings"

// rewriteBareTerms turns words without an operator into subject searches,
// since Gerrit would otherwise try them as change numbers or ids. Each word
// must match somewhere in the subject; a quoted string must match as a
// phrase: `is:open dhcp meter` → `is:open subject:dhcp subject:meter`.
func rewriteBareTerms(query string) string {
	var out []string
	for _, tok := range tokenize(query) {
		if isBareTerm(tok) {
			tok = "subject:" + tok
		}
		out = append(out, tok)
	}
	return strings.Join(out, " ")
}

func isBareTerm(tok string) bool {
	switch {
	case tok == "AND", tok == "OR", tok == "NOT", tok == "(", tok == ")":
		return false
	case strings.HasPrefix(tok, "-"), strings.Contains(tok, ":"):
		return false
	}
	return true
}

// tokenize splits on spaces, keeping quoted strings and operator values
// together and separating parentheses.
func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	emit := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case inQuote:
			cur.WriteRune(r)
		case r == ' ' || r == '\t':
			emit()
		case r == '(' || r == ')':
			emit()
			tokens = append(tokens, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	emit()
	return tokens
}
