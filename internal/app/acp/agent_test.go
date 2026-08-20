package acpbridge

import (
	"context"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

func TestPromptForwardsTextAndStreamsTutorResponse(t *testing.T) {
	var got request.Request
	agent := New(func(_ context.Context, req request.Request) (response.Response, error) {
		got = req
		return response.Response{Message: "What do you predict?"}, nil
	})

	var update sdk.SessionNotification
	agent.update = func(_ context.Context, notification sdk.SessionNotification) error {
		update = notification
		return nil
	}

	session, err := agent.NewSession(context.Background(), sdk.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []sdk.McpServer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := agent.Prompt(context.Background(), sdk.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []sdk.ContentBlock{sdk.TextBlock("Why does this loop stop?")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != sdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", result.StopReason)
	}
	if got.Message != "Why does this loop stop?" || got.SessionID != string(session.SessionId) {
		t.Fatalf("forwarded request = %+v", got)
	}
	if update.Update.AgentMessageChunk == nil || update.Update.AgentMessageChunk.Content.Text.Text != "What do you predict?" {
		t.Fatalf("streamed update = %+v", update)
	}
}
