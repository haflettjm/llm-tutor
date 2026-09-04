package acpbridge

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialWithRetryRetriesConnectionFailures(t *testing.T) {
	var attempts int
	client, server := net.Pipe()
	defer server.Close()

	conn, err := dialWithRetry(context.Background(), func(context.Context, string, string) (net.Conn, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("connection refused")
		}
		return client, nil
	}, "unix", "test")
	if err != nil {
		t.Fatalf("dialWithRetry: %v", err)
	}
	defer conn.Close()
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClientGivesUpWithClearSocketError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewHTTPClient("/tmp/definitely-not-a-socket.sock").Progress(ctx)
	if err == nil || !strings.Contains(err.Error(), "definitely-not-a-socket.sock") {
		t.Fatalf("err = %v", err)
	}
}
