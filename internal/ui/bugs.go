package ui

import (
	"regexp"
	"strings"
)

type bugRef struct {
	kind string // Closes, Partial, Related, ...
	id   string
}

// bugFooter matches OpenStack-style footers such as "Closes-Bug: #2170284".
var bugFooter = regexp.MustCompile(`(?im)^\s*([A-Za-z]+)-Bug:\s*#?\s*(\d+)\s*$`)

func parseBugRefs(message string) []bugRef {
	var refs []bugRef
	seen := map[string]bool{}
	for _, m := range bugFooter.FindAllStringSubmatch(message, -1) {
		kind := strings.ToLower(m[1])
		kind = strings.ToUpper(kind[:1]) + kind[1:]
		key := kind + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, bugRef{kind: kind, id: m[2]})
	}
	return refs
}

func bugURL(template, id string) string {
	return strings.ReplaceAll(template, "{id}", id)
}
