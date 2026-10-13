package gerrit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// xssiPrefix is prepended by Gerrit to every JSON body and must be stripped.
var xssiPrefix = []byte(")]}'")

type Client struct {
	baseURL  string
	username string
	token    string
	http     *http.Client
}

func New(baseURL, username, token string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		token:    token,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) Authenticated() bool { return c.username != "" && c.token != "" }

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("gerrit: %d %s", e.Status, strings.TrimSpace(e.Body))
}

func IsNotFound(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusNotFound
}

func (c *Client) endpoint(path string, query url.Values) string {
	u := c.baseURL
	if c.Authenticated() {
		u += "/a"
	}
	u += path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// send runs a request with credentials and returns the body, or an
// HTTPError for non-2xx responses.
func (c *Client) send(req *http.Request) ([]byte, error) {
	if c.Authenticated() {
		req.SetBasicAuth(c.username, c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(raw)}
	}
	return raw, nil
}

// do sends in as JSON and decodes the JSON response into out when given.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}
	raw, err := c.send(req)
	if err != nil || out == nil || len(raw) == 0 {
		return err
	}
	return json.Unmarshal(bytes.TrimPrefix(raw, xssiPrefix), out)
}

// getRaw fetches a non-JSON endpoint. Gerrit serves file contents and patches
// as base64 text, so no Accept header is sent and the body is returned as is.
func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path, query), nil)
	if err != nil {
		return nil, err
	}
	return c.send(req)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, in, out)
}
