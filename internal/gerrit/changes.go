package gerrit

import (
	"context"
	"net/url"
	"strconv"
)

type QueryOptions struct {
	Limit   int
	Start   int
	Options []string
}

func (c *Client) QueryChanges(ctx context.Context, query string, opts QueryOptions) ([]ChangeInfo, error) {
	q := optionsQuery(opts.Options...)
	q.Set("q", query)
	if opts.Limit > 0 {
		q.Set("n", strconv.Itoa(opts.Limit))
	}
	if opts.Start > 0 {
		q.Set("S", strconv.Itoa(opts.Start))
	}
	var changes []ChangeInfo
	if err := c.get(ctx, "/changes/", q, &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

// ChangeID builds the canonical "<project>~<number>" identifier, URL-encoded
// as Gerrit expects it in paths.
func ChangeID(project string, number int) string {
	return url.PathEscape(project) + "~" + strconv.Itoa(number)
}

// ChangeURL is the web page of a change.
func (c *Client) ChangeURL(project string, number int) string {
	return c.baseURL + "/c/" + project + "/+/" + strconv.Itoa(number)
}

// GetChange fetches a change; id must already be URL-encoded (see ChangeID).
// Extra o= options may be passed.
func (c *Client) GetChange(ctx context.Context, id string, options ...string) (*ChangeInfo, error) {
	var change ChangeInfo
	if err := c.get(ctx, "/changes/"+id, optionsQuery(options...), &change); err != nil {
		return nil, err
	}
	return &change, nil
}

// GetChangeDetail is GetChange with labels, detailed accounts, messages and
// reviewers included.
func (c *Client) GetChangeDetail(ctx context.Context, id string, options ...string) (*ChangeInfo, error) {
	var change ChangeInfo
	if err := c.get(ctx, "/changes/"+id+"/detail", optionsQuery(options...), &change); err != nil {
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

func optionsQuery(options ...string) url.Values {
	q := url.Values{}
	for _, o := range options {
		q.Add("o", o)
	}
	return q
}
