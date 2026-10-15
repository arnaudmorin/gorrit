package gerrit

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/url"
	"strconv"
)

// RevisionCurrent names the latest patch set in revision endpoints.
const RevisionCurrent = "current"

func revisionPath(id, rev string) string {
	return "/changes/" + id + "/revisions/" + url.PathEscape(rev)
}

// ListFiles lists the files of a revision against its parent, or against
// patch set base when base is 1 or more.
func (c *Client) ListFiles(ctx context.Context, id, rev string, base int) (map[string]FileInfo, error) {
	var files map[string]FileInfo
	if err := c.get(ctx, revisionPath(id, rev)+"/files/", baseQuery(base), &files); err != nil {
		return nil, err
	}
	return files, nil
}

// GetDiff returns the diff of one file of a revision against its parent, or
// against patch set base when base is 1 or more.
func (c *Client) GetDiff(ctx context.Context, id, rev, path string, base int) (*DiffInfo, error) {
	var d DiffInfo
	if err := c.get(ctx, revisionPath(id, rev)+"/files/"+url.PathEscape(path)+"/diff", baseQuery(base), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func baseQuery(base int) url.Values {
	if base < 1 {
		return nil
	}
	return url.Values{"base": {strconv.Itoa(base)}}
}

func decodeBase64(raw []byte) ([]byte, error) {
	return io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(raw)))
}

// GetFileContent returns a file at the given revision, or at its parent when
// parent is 1 or more. ok is false when the file does not exist on that side.
func (c *Client) GetFileContent(ctx context.Context, id, rev, path string, parent int) (content []byte, ok bool, err error) {
	q := url.Values{}
	if parent > 0 {
		q.Set("parent", strconv.Itoa(parent))
	}
	raw, err := c.getRaw(ctx, revisionPath(id, rev)+"/files/"+url.PathEscape(path)+"/content", q)
	if IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	content, err = decodeBase64(raw)
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}

// GetPatch returns the revision as a git format-patch.
func (c *Client) GetPatch(ctx context.Context, id, rev string) ([]byte, error) {
	raw, err := c.getRaw(ctx, revisionPath(id, rev)+"/patch", nil)
	if err != nil {
		return nil, err
	}
	return decodeBase64(raw)
}
