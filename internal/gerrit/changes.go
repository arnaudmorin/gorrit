package gerrit

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

type QueryOptions struct {
	Limit   int
	Start   int
	Options []string
}

// QueryChanges runs one or more queries and returns one result list per query,
// in the order given. Gerrit returns a flat list for a single query and a list
// of lists otherwise.
func (c *Client) QueryChanges(ctx context.Context, queries []string, opts QueryOptions) ([][]ChangeInfo, error) {
	q := url.Values{}
	for _, query := range queries {
		q.Add("q", query)
	}
	if opts.Limit > 0 {
		q.Set("n", strconv.Itoa(opts.Limit))
	}
	if opts.Start > 0 {
		q.Set("S", strconv.Itoa(opts.Start))
	}
	for _, o := range opts.Options {
		q.Add("o", o)
	}
	var raw json.RawMessage
	if err := c.get(ctx, "/changes/", q, &raw); err != nil {
		return nil, err
	}
	if len(queries) == 1 {
		var single []ChangeInfo
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, err
		}
		return [][]ChangeInfo{single}, nil
	}
	var multi [][]ChangeInfo
	if err := json.Unmarshal(raw, &multi); err != nil {
		return nil, err
	}
	return multi, nil
}

// ChangeID builds the canonical "<project>~<number>" identifier, URL-encoded
// as Gerrit expects it in paths.
func ChangeID(project string, number int) string {
	return url.PathEscape(project) + "~" + strconv.Itoa(number)
}

// GetChangeDetail fetches a change with labels, detailed accounts, messages
// and reviewers. id must already be URL-encoded (see ChangeID). Extra o=
// options may be passed.
func (c *Client) GetChangeDetail(ctx context.Context, id string, options ...string) (*ChangeInfo, error) {
	q := url.Values{}
	for _, o := range options {
		q.Add("o", o)
	}
	var change ChangeInfo
	if err := c.get(ctx, "/changes/"+id+"/detail", q, &change); err != nil {
		return nil, err
	}
	return &change, nil
}

func (c *Client) GetSelf(ctx context.Context) (*AccountInfo, error) {
	var acct AccountInfo
	if err := c.get(ctx, "/accounts/self", nil, &acct); err != nil {
		return nil, err
	}
	return &acct, nil
}
