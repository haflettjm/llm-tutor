package acpbridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type Query func(context.Context, request.Request) (response.Response, error)

type Agent struct {
	conn   *sdk.AgentSideConnection
	query  Query
	update func(context.Context, sdk.SessionNotification) error
}

func New(query Query) *Agent {
	return &Agent{query: query}
}

func (a *Agent) SetAgentConnection(conn *sdk.AgentSideConnection) {
	a.conn = conn
	a.update = conn.SessionUpdate
}

func (a *Agent) Initialize(_ context.Context, params sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	version := params.ProtocolVersion
	if version > sdk.ProtocolVersionNumber {
		version = sdk.ProtocolVersionNumber
	}
	return sdk.InitializeResponse{
		ProtocolVersion: version,
		AgentInfo: &sdk.Implementation{
			Name:    "knumble-tutor",
			Title:   sdk.Ptr("Knumble Tutor"),
			Version: "0.1.0",
		},
	}, nil
}

func (a *Agent) NewSession(_ context.Context, _ sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return sdk.NewSessionResponse{}, fmt.Errorf("create session id: %w", err)
	}
	return sdk.NewSessionResponse{SessionId: sdk.SessionId(hex.EncodeToString(id[:]))}, nil
}

func (a *Agent) Prompt(ctx context.Context, params sdk.PromptRequest) (sdk.PromptResponse, error) {
	message := promptText(params.Prompt)
	if message == "" {
		return sdk.PromptResponse{}, fmt.Errorf("prompt has no text")
	}
	if a.update == nil {
		return sdk.PromptResponse{}, fmt.Errorf("ACP connection is not ready")
	}

	resp, err := a.query(ctx, request.Request{
		Message:   message,
		SessionID: string(params.SessionId),
	})
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	if err := a.update(ctx, sdk.SessionNotification{
		SessionId: params.SessionId,
		Update:    sdk.UpdateAgentMessageText(resp.Message),
	}); err != nil {
		return sdk.PromptResponse{}, err
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func promptText(blocks []sdk.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != nil {
			parts = append(parts, block.Text.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (a *Agent) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, nil
}

func (a *Agent) Cancel(context.Context, sdk.CancelNotification) error { return nil }

func (a *Agent) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionClose)
}

func (a *Agent) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionList)
}

func (a *Agent) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodLogout)
}

func (a *Agent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionResume)
}

func (a *Agent) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionSetConfigOption)
}

func (a *Agent) SetSessionMode(context.Context, sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionSetMode)
}

func Serve(query Query, stdout io.Writer, stdin io.Reader) {
	agent := New(query)
	conn := sdk.NewAgentSideConnection(agent, stdout, stdin)
	agent.SetAgentConnection(conn)
	<-conn.Done()
}

func UnixSocketQuery(socket string) Query {
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}

	return func(ctx context.Context, req request.Request) (response.Response, error) {
		body, err := json.Marshal(req)
		if err != nil {
			return response.Response{}, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/tutor", bytes.NewReader(body))
		if err != nil {
			return response.Response{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		httpResp, err := client.Do(httpReq)
		if err != nil {
			return response.Response{}, fmt.Errorf("connect to tutor daemon at %s: %w", socket, err)
		}
		defer httpResp.Body.Close()
		if httpResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
			return response.Response{}, fmt.Errorf("tutor daemon returned %s: %s", httpResp.Status, strings.TrimSpace(string(body)))
		}

		var resp response.Response
		if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
			return response.Response{}, fmt.Errorf("decode tutor response: %w", err)
		}
		return resp, nil
	}
}
