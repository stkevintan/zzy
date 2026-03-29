package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/a3tai/openclaw-go/gateway"
	"github.com/a3tai/openclaw-go/protocol"
)

// Client wraps an OpenClaw gateway connection for chat.
type Client struct {
	host  string
	token string

	mu     sync.Mutex
	client *gateway.Client

	chatMu   sync.Mutex
	chatDone map[string]chan chatResult
}

type chatResult struct {
	text string
	err  error
}

func NewClient(host, token string) *Client {
	return &Client{
		host:     host,
		token:    token,
		chatDone: make(map[string]chan chatResult),
	}
}

// Connect establishes the WebSocket connection to the gateway.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return nil
	}

	client := gateway.NewClient(
		gateway.WithToken(c.token),
		gateway.WithRole(protocol.RoleOperator),
		gateway.WithScopes(protocol.ScopeOperatorRead, protocol.ScopeOperatorWrite),
		gateway.WithOnEvent(c.handleEvent),
	)

	if err := client.Connect(ctx, c.host); err != nil {
		return fmt.Errorf("openclaw: connect: %w", err)
	}

	hello := client.Hello()
	slog.Info("openclaw connected", "protocol", hello.Protocol, "server", hello.Server.Version)

	c.client = client
	return nil
}

// Close shuts down the gateway connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}

// Chat sends a message and waits for the complete response.
func (c *Client) Chat(ctx context.Context, sessionKey, message string) (string, error) {
	if err := c.Connect(ctx); err != nil {
		return "", err
	}

	idempotencyKey := fmt.Sprintf("chat-%d", time.Now().UnixNano())

	// Register a channel to collect the response
	ch := make(chan chatResult, 1)
	c.chatMu.Lock()
	c.chatDone[idempotencyKey] = ch
	c.chatMu.Unlock()
	defer func() {
		c.chatMu.Lock()
		delete(c.chatDone, idempotencyKey)
		c.chatMu.Unlock()
	}()

	_, err := c.client.ChatSend(ctx, protocol.ChatSendParams{
		SessionKey:     sessionKey,
		Message:        message,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", fmt.Errorf("openclaw: chat send: %w", err)
	}

	// Wait for the final event or context cancellation
	select {
	case result := <-ch:
		return result.text, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ResetSession resets a chat session.
func (c *Client) ResetSession(ctx context.Context, sessionKey string) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	return c.client.SessionsReset(ctx, protocol.SessionsResetParams{Key: sessionKey})
}

func (c *Client) handleEvent(ev protocol.Event) {
	if ev.EventName != protocol.EventChat {
		return
	}

	var data struct {
		State        string `json:"state"`
		Message      string `json:"message"`
		ErrorMessage string `json:"errorMessage"`
		SessionKey   string `json:"sessionKey"`
	}
	if err := json.Unmarshal(ev.Payload, &data); err != nil {
		slog.Warn("openclaw: failed to parse chat event", "error", err)
		return
	}

	switch data.State {
	case "delta":
		c.accumulateDelta(data.SessionKey, data.Message)
	case "final":
		c.finalize(data.SessionKey, "", nil)
	case "error":
		c.finalize(data.SessionKey, "", fmt.Errorf("openclaw chat error: %s", data.ErrorMessage))
	}
}

var (
	deltasMu sync.Mutex
	deltas   = make(map[string]*strings.Builder)
)

func (c *Client) accumulateDelta(sessionKey, msg string) {
	deltasMu.Lock()
	defer deltasMu.Unlock()
	b, ok := deltas[sessionKey]
	if !ok {
		b = &strings.Builder{}
		deltas[sessionKey] = b
	}
	b.WriteString(msg)
}

func (c *Client) finalize(sessionKey string, _ string, err error) {
	deltasMu.Lock()
	b := deltas[sessionKey]
	text := ""
	if b != nil {
		text = b.String()
		delete(deltas, sessionKey)
	}
	deltasMu.Unlock()

	// Signal all pending chat requests for this session
	c.chatMu.Lock()
	for key, ch := range c.chatDone {
		// Send to first matching pending request
		select {
		case ch <- chatResult{text: text, err: err}:
			delete(c.chatDone, key)
		default:
		}
		break
	}
	c.chatMu.Unlock()
}
