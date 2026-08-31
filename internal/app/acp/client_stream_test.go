package acpbridge

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/haflettjm/llm-tutor/internal/types/request"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestQueryStreamParsesSSE(t *testing.T) {
	c := streamTestClient("event: chunk\ndata: {\"text\":\"one \"}\n\nevent: chunk\ndata: {\"text\":\"two\"}\n\nevent: done\ndata: {\"message\":\"one two\",\"response_type\":\"question\"}\n\n")
	var got strings.Builder
	resp, err := c.QueryStream(context.Background(), request.Request{Message: "hi"}, func(text string, reset bool) error {
		if reset {
			got.Reset()
		}
		got.WriteString(text)
		return nil
	})
	if err != nil || got.String() != "one two" || resp.Message != "one two" {
		t.Fatalf("stream=%q response=%+v error=%v", got.String(), resp, err)
	}
}

func TestQueryStreamReturnsErrorEvent(t *testing.T) {
	_, err := streamTestClient("event: error\ndata: {\"error\":\"model refused\"}\n\n").QueryStream(context.Background(), request.Request{Message: "hi"}, func(string, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "model refused") {
		t.Fatalf("err = %v", err)
	}
}

func streamTestClient(body string) *httpClient {
	return &httpClient{socket: "test", http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}
