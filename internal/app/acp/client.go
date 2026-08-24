package acpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
	"github.com/haflettjm/llm-tutor/internal/types/status"
)

// Client is the bridge's view of the tutor daemon.
//
// Query costs a model turn; the others answer from the daemon's local state.
// Keeping them separate is the point: "where am I in the track" must not spend
// an API call to read a JSON file.
type Client interface {
	Query(ctx context.Context, req request.Request) (response.Response, error)
	Progress(ctx context.Context) (status.Progress, error)
	Plans(ctx context.Context) (status.Plans, error)
	SetTrack(ctx context.Context, track string) (status.Progress, error)
}

// httpClient talks to the daemon over its unix socket.
type httpClient struct {
	socket string
	http   *http.Client
}

// NewHTTPClient returns a Client bound to the daemon listening on socket.
func NewHTTPClient(socket string) Client {
	return &httpClient{
		socket: socket,
		http: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		}},
	}
}

func (c *httpClient) Query(ctx context.Context, req request.Request) (response.Response, error) {
	var out response.Response
	err := c.do(ctx, http.MethodPost, "/tutor", req, &out)
	return out, err
}

func (c *httpClient) Progress(ctx context.Context) (status.Progress, error) {
	var out status.Progress
	err := c.do(ctx, http.MethodGet, "/progress", nil, &out)
	return out, err
}

func (c *httpClient) Plans(ctx context.Context) (status.Plans, error) {
	var out status.Plans
	err := c.do(ctx, http.MethodGet, "/plans", nil, &out)
	return out, err
}

func (c *httpClient) SetTrack(ctx context.Context, track string) (status.Progress, error) {
	var out status.Progress
	err := c.do(ctx, http.MethodPost, "/track", status.TrackRequest{Track: track}, &out)
	return out, err
}

// do performs one request against the daemon, decoding the daemon's structured
// error body when there is one so the learner sees the real reason.
func (c *httpClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("connect to tutor daemon at %s: %w", c.socket, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		var e status.Error
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return fmt.Errorf("tutor daemon: %s", e.Error)
		}
		return fmt.Errorf("tutor daemon returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode tutor response: %w", err)
	}
	return nil
}
