package gerrit

import (
	"context"
	"net/http"
)

// SetStarred stars or unstars a change for the calling user.
func (c *Client) SetStarred(ctx context.Context, id string, starred bool) error {
	method := http.MethodPut
	if !starred {
		method = http.MethodDelete
	}
	return c.do(ctx, method, "/accounts/self/starred.changes/"+id, nil, nil, nil)
}
