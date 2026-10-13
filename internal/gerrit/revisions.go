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

func (c *Client) ListFiles(ctx context.Context, id, rev string) (map[string]FileInfo, error) {
	var files map[string]FileInfo
	if err := c.get(ctx, revisionPath(id, rev)+"/files/", nil, &files); err != nil {
		return nil, err
	}
	return files, nil
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
	return content, err == nil, err
}

// GetPatch returns the revision as a git format-patch.
func (c *Client) GetPatch(ctx context.Context, id, rev string) ([]byte, error) {
	raw, err := c.getRaw(ctx, revisionPath(id, rev)+"/patch", nil)
	if err != nil {
		return nil, err
	}
	return decodeBase64(raw)
}
