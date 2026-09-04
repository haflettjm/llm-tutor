package acpbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

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
	QueryStream(ctx context.Context, req request.Request, onChunk func(text string, reset bool) error) (response.Response, error)
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
				return dialWithRetry(ctx, (&net.Dialer{}).DialContext, "unix", socket)
			},
		}},
	}
}

const (
	dialAttempts  = 3
	dialRetryWait = 200 * time.Millisecond
)

// dialWithRetry covers the brief gap while systemd restarts the local daemon.
// It wraps only DialContext, never an HTTP request, so an accepted tutor turn is
// not accidentally replayed and billed twice.
func dialWithRetry(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error), network, address string) (net.Conn, error) {
	var lastErr error
	for attempt := 1; attempt <= dialAttempts; attempt++ {
		conn, err := dial(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if attempt == dialAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("dial tutor daemon at %s: %w", address, ctx.Err())
		case <-time.After(dialRetryWait):
		}
	}
	return nil, fmt.Errorf("dial tutor daemon at %s: %w", address, lastErr)
}

func (c *httpClient) Query(ctx context.Context, req request.Request) (response.Response, error) {
	var out response.Response
	err := c.do(ctx, http.MethodPost, "/tutor", req, &out)
	return out, err
}

func (c *httpClient) QueryStream(ctx context.Context, in request.Request, onChunk func(string, bool) error) (response.Response, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return response.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/tutor/stream", bytes.NewReader(data))
	if err != nil {
		return response.Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return response.Response{}, fmt.Errorf("connect to tutor daemon at %s: %w", c.socket, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response.Response{}, fmt.Errorf("tutor daemon returned %s", resp.Status)
	}
	return parseSSE(resp.Body, onChunk)
}

func parseSSE(r io.Reader, onChunk func(string, bool) error) (response.Response, error) {
	var event, data string
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			out, err := handleSSE(event, data, onChunk)
			if err != nil {
				return response.Response{}, err
			}
			if event == "done" {
				return out, nil
			}
			event, data = "", ""
			continue
		}
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			event = after
		}
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			data = after
		}
	}
	return response.Response{}, s.Err()
}

func handleSSE(event, data string, onChunk func(string, bool) error) (response.Response, error) {
	switch event {
	case "chunk":
		var chunk struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return response.Response{}, err
		}
		return response.Response{}, onChunk(chunk.Text, false)
	case "reset":
		return response.Response{}, onChunk("", true)
	case "done":
		var out response.Response
		return out, json.Unmarshal([]byte(data), &out)
	case "error":
		var out status.Error
		_ = json.Unmarshal([]byte(data), &out)
		return response.Response{}, fmt.Errorf("tutor daemon: %s", out.Error)
	}
	return response.Response{}, nil
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
