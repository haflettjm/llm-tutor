// Package acpbridge adapts the tutor daemon to the Agent Client Protocol so
// editors (Zed's agent panel, Neovim ACP clients) can drive it directly.
//
// The bridge is deliberately thin and stateless: it is spawned per workspace by
// the editor and dies with the window, while the daemon it relays to outlives
// both and owns all learner state.
package acpbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/haflettjm/llm-tutor/internal/types/request"
)

// Agent implements the ACP agent side.
type Agent struct {
	conn   *sdk.AgentSideConnection
	client Client
	update func(context.Context, sdk.SessionNotification) error

	// inflight lets session/cancel actually stop the turn it names, rather
	// than acknowledging the cancel and letting the model run on.
	mu       sync.Mutex
	inflight map[sdk.SessionId]context.CancelFunc
}

// New returns an Agent relaying to the given tutor daemon.
func New(client Client) *Agent {
	return &Agent{client: client, inflight: make(map[sdk.SessionId]context.CancelFunc)}
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

// NewSession mints a session id and publishes the command menu. The ACP
// connection holds notifications sent from inside a request handler until the
// response is written, so the client always has the session id before the
// update naming it arrives.
func (a *Agent) NewSession(ctx context.Context, _ sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return sdk.NewSessionResponse{}, fmt.Errorf("create session id: %w", err)
	}
	sessionID := sdk.SessionId(hex.EncodeToString(id[:]))

	if a.update != nil {
		// A client that cannot render the menu is not a reason to fail the
		// session -- every command also works as plain text.
		_ = a.update(ctx, sdk.SessionNotification{
			SessionId: sessionID,
			Update: sdk.SessionUpdate{
				AvailableCommandsUpdate: &sdk.SessionAvailableCommandsUpdate{
					AvailableCommands: availableCommands(),
				},
			},
		})
	}
	return sdk.NewSessionResponse{SessionId: sessionID}, nil
}

// Prompt handles one turn: a slash command, or a question for the tutor.
func (a *Agent) Prompt(ctx context.Context, params sdk.PromptRequest) (sdk.PromptResponse, error) {
	message := promptText(params.Prompt)
	if message == "" {
		return sdk.PromptResponse{}, fmt.Errorf("prompt has no text")
	}
	if a.update == nil {
		return sdk.PromptResponse{}, fmt.Errorf("ACP connection is not ready")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.track(params.SessionId, cancel)
	defer a.untrack(params.SessionId)

	reply, err := a.respond(ctx, params.SessionId, message)
	if err != nil {
		if ctx.Err() != nil {
			return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
		}
		// Surface the failure in the transcript as well as returning it: an
		// editor that renders errors quietly would otherwise leave the learner
		// staring at a prompt that did nothing.
		_ = a.send(ctx, params.SessionId, "The tutor could not answer: "+err.Error())
		return sdk.PromptResponse{}, err
	}

	if err := a.send(ctx, params.SessionId, reply); err != nil {
		return sdk.PromptResponse{}, err
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

// respond produces the reply text for one turn.
func (a *Agent) respond(ctx context.Context, session sdk.SessionId, message string) (string, error) {
	if cmd, args, ok := parseCommand(message); ok {
		if cmd.local != nil {
			return cmd.local(ctx, a, args)
		}
		message = cmd.directive(args)
	}

	resp, err := a.client.Query(ctx, request.Request{
		Message:   message,
		SessionID: string(session),
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Message) == "" {
		return "", errors.New("the tutor returned an empty reply")
	}
	return resp.Message, nil
}

func (a *Agent) send(ctx context.Context, session sdk.SessionId, text string) error {
	return a.update(ctx, sdk.SessionNotification{
		SessionId: session,
		Update:    sdk.UpdateAgentMessageText(text),
	})
}

func (a *Agent) track(session sdk.SessionId, cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// A new prompt on a session supersedes whatever was still running on it.
	if prev, ok := a.inflight[session]; ok {
		prev()
	}
	a.inflight[session] = cancel
}

func (a *Agent) untrack(session sdk.SessionId) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inflight, session)
}

// Cancel stops the in-flight turn for a session, if there is one.
func (a *Agent) Cancel(_ context.Context, params sdk.CancelNotification) error {
	a.mu.Lock()
	cancel, ok := a.inflight[params.SessionId]
	a.mu.Unlock()
	if ok {
		cancel()
	}
	return nil
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

func (a *Agent) CloseSession(_ context.Context, params sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	a.mu.Lock()
	cancel, ok := a.inflight[params.SessionId]
	delete(a.inflight, params.SessionId)
	a.mu.Unlock()
	if ok {
		cancel()
	}
	return sdk.CloseSessionResponse{}, nil
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

// Serve runs the ACP agent loop over stdio until the connection closes.
func Serve(client Client, stdout io.Writer, stdin io.Reader) {
	agent := New(client)
	conn := sdk.NewAgentSideConnection(agent, stdout, stdin)
	agent.SetAgentConnection(conn)
	<-conn.Done()
}
