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
	"github.com/a3tai/openclaw-go/identity"
	"github.com/a3tai/openclaw-go/protocol"
)

// Client wraps an OpenClaw gateway connection for chat.
type Client struct {
	host        string
	token       string
	identityDir string

	mu     sync.Mutex
	client *gateway.Client

	chatMu   sync.Mutex
	chatDone map[string]chan chatResult
}

type chatResult struct {
	text string
	err  error
}

func NewClient(host, token, identityDir string) *Client {
	return &Client{
		host:        host,
		token:       token,
		identityDir: identityDir,
		chatDone:    make(map[string]chan chatResult),
	}
}

// Connect establishes the WebSocket connection to the gateway.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return nil
	}

	opts := []gateway.Option{
		gateway.WithToken(c.token),
		gateway.WithRole(protocol.RoleOperator),
		gateway.WithScopes(protocol.ScopeOperatorRead, protocol.ScopeOperatorWrite, protocol.ScopeOperatorAdmin),
		gateway.WithOnEvent(c.handleEvent),
	}

	// Load or generate device identity so the gateway preserves our scopes.
	if c.identityDir != "" {
		store, err := identity.NewStore(c.identityDir)
		if err != nil {
			return fmt.Errorf("openclaw: identity store: %w", err)
		}
		id, err := store.LoadOrGenerate()
		if err != nil {
			return fmt.Errorf("openclaw: load identity: %w", err)
		}
		deviceToken := store.LoadDeviceToken()
		opts = append(opts, gateway.WithIdentity(id, deviceToken))
		slog.Debug("openclaw identity loaded", "device_id", id.DeviceID)
	}

	client := gateway.NewClient(opts...)

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

	var data map[string]any
	if json.Unmarshal(ev.Payload, &data) != nil {
		return
	}

	sessionKey, _ := data["sessionKey"].(string)
	state, _ := data["state"].(string)

	switch state {
	case "delta":
		msg := extractText(data["message"])
		if msg != "" {
			c.accumulateDelta(sessionKey, msg)
		}
	case "final":
		c.finalize(sessionKey, nil)
	case "error":
		errMsg, _ := data["errorMessage"].(string)
		c.finalize(sessionKey, fmt.Errorf("openclaw chat error: %s", errMsg))
	}
}

// extractText extracts plain text from a message value that can be:
//   - a string: "hello"
//   - an object: {"role":"assistant","content":[{"type":"text","text":"hello"}]}
func extractText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	content := obj["content"]
	// content can be a string
	if s, ok := content.(string); ok {
		return s
	}
	// content can be an array of parts
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := part["type"].(string); t == "text" {
			if text, ok := part["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

var (
	deltasMu sync.Mutex
	deltas   = make(map[string]string)
)

func (c *Client) accumulateDelta(sessionKey, msg string) {
	deltasMu.Lock()
	defer deltasMu.Unlock()
	deltas[sessionKey] = msg
}

func (c *Client) finalize(sessionKey string, err error) {
	deltasMu.Lock()
	text := deltas[sessionKey]
	delete(deltas, sessionKey)
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
