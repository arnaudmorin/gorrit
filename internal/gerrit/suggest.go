package gerrit

import (
	"context"
	"net/url"
	"sort"
	"strconv"
)

// SuggestProjects lists project names starting with prefix.
func (c *Client) SuggestProjects(ctx context.Context, prefix string, limit int) ([]string, error) {
	q := url.Values{"p": {prefix}, "n": {strconv.Itoa(limit)}}
	var projects map[string]struct{}
	if err := c.get(ctx, "/projects/", q, &projects); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(projects))
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// SuggestAccounts returns accounts matching text, as Gerrit's own
// autocomplete does.
func (c *Client) SuggestAccounts(ctx context.Context, text string, limit int) ([]AccountInfo, error) {
	q := url.Values{"suggest": {""}, "q": {text}, "n": {strconv.Itoa(limit)}}
	var accounts []AccountInfo
	if err := c.get(ctx, "/accounts/", q, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}
