package gerrit

import (
	"context"
	"net/url"
)

type CommentRange struct {
	StartLine      int `json:"start_line"`
	StartCharacter int `json:"start_character"`
	EndLine        int `json:"end_line"`
	EndCharacter   int `json:"end_character"`
}

type ContextLine struct {
	LineNumber int    `json:"line_number"`
	Text       string `json:"context_line"`
}

type CommentInfo struct {
	PatchSet   int           `json:"patch_set"`
	ID         string        `json:"id"`
	Path       string        `json:"path"`
	Side       string        `json:"side"`
	Line       int           `json:"line"`
	Range      *CommentRange `json:"range"`
	InReplyTo  string        `json:"in_reply_to"`
	Message    string        `json:"message"`
	Updated    Timestamp     `json:"updated"`
	Author     *AccountInfo  `json:"author"`
	Tag        string        `json:"tag"`
	Unresolved bool          `json:"unresolved"`
	CommitID   string        `json:"commit_id"`
	Context    []ContextLine `json:"context_lines"`
}

// CommentInput creates or replies to an inline comment.
type CommentInput struct {
	Path       string        `json:"path,omitempty"`
	Side       string        `json:"side,omitempty"`
	Line       int           `json:"line,omitempty"`
	Range      *CommentRange `json:"range,omitempty"`
	InReplyTo  string        `json:"in_reply_to,omitempty"`
	Message    string        `json:"message"`
	Unresolved *bool         `json:"unresolved,omitempty"`
}

type ReviewInput struct {
	Message  string                    `json:"message,omitempty"`
	Labels   map[string]int            `json:"labels,omitempty"`
	Comments map[string][]CommentInput `json:"comments,omitempty"`
	Notify   string                    `json:"notify,omitempty"`
}

type ReviewResult struct {
	Labels map[string]int `json:"labels"`
}

// SetReview posts a review on a revision: comments, votes and a message in
// one request. rev is a patch set number, commit or "current".
func (c *Client) SetReview(ctx context.Context, id, rev string, in ReviewInput) (*ReviewResult, error) {
	var out ReviewResult
	if err := c.post(ctx, revisionPath(id, rev)+"/review", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListComments returns the published inline comments of all patch sets,
// keyed by file path, each with its patch_set and the surrounding source
// lines.
func (c *Client) ListComments(ctx context.Context, id string) (map[string][]CommentInfo, error) {
	q := url.Values{"enable-context": {"true"}, "context-padding": {"2"}}
	var comments map[string][]CommentInfo
	if err := c.get(ctx, "/changes/"+id+"/comments", q, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

// GetChange fetches a change without the detail extras.
func (c *Client) GetChange(ctx context.Context, id string, options ...string) (*ChangeInfo, error) {
	q := url.Values{}
	for _, o := range options {
		q.Add("o", o)
	}
	var change ChangeInfo
	if err := c.get(ctx, "/changes/"+id, q, &change); err != nil {
		return nil, err
	}
	return &change, nil
}
