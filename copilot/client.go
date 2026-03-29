package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	defaultModel    = "gpt-4o"
	tokenURL        = "https://api.github.com/copilot_internal/v2/token"
	defaultBaseURL  = "https://api.githubcopilot.com"
	tokenSafeMargin = 5 * time.Minute
)

// Client is a GitHub Copilot API client that automatically manages
// short-lived Copilot API tokens exchanged from a long-lived GitHub token.
type Client struct {
	githubToken string
	model       string
	httpClient  *http.Client

	mu        sync.Mutex
	apiToken  string
	baseURL   string
	expiresAt time.Time
}

// Option configures the Client.
type Option func(*Client)

func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient creates a new Copilot API client.
// githubToken is the long-lived GitHub personal access token (e.g. from `gh auth token`).
// The client automatically exchanges it for short-lived Copilot API tokens.
func NewClient(githubToken string, opts ...Option) *Client {
	c := &Client{
		githubToken: githubToken,
		model:       defaultModel,
		httpClient:  http.DefaultClient,
		baseURL:     defaultBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// copilotTokenResponse is the response from the Copilot token exchange endpoint.
type copilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// resolveToken returns a valid Copilot API token, refreshing if needed.
func (c *Client) resolveToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.apiToken != "" && time.Now().Before(c.expiresAt.Add(-tokenSafeMargin)) {
		return c.apiToken, nil
	}

	slog.Debug("exchanging github token for copilot api token")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("copilot: create token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.githubToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("copilot: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("copilot: read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copilot: token exchange failed (HTTP %d): %s", resp.StatusCode, body)
	}

	var tokenResp copilotTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("copilot: unmarshal token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("copilot: token response missing token")
	}

	c.apiToken = tokenResp.Token
	// GitHub returns Unix seconds; handle both seconds and milliseconds.
	if tokenResp.ExpiresAt < 100_000_000_000 {
		c.expiresAt = time.Unix(tokenResp.ExpiresAt, 0)
	} else {
		c.expiresAt = time.UnixMilli(tokenResp.ExpiresAt)
	}

	slog.Debug("copilot api token acquired", "expires_at", c.expiresAt)
	return c.apiToken, nil
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message Message `json:"message"`
}

// Chat sends messages to the completions API and returns the text response.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
	}
	return c.do(ctx, reqBody)
}

// Parse sends a system prompt and user content to the completions API with
// JSON mode enabled, and unmarshals the response into type T.
func Parse[T any](ctx context.Context, c *Client, systemPrompt, userContent string) (*T, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	respMsg, err := c.do(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var result T
	if err := json.Unmarshal([]byte(respMsg), &result); err != nil {
		return nil, fmt.Errorf("copilot: unmarshal result: %w", err)
	}
	return &result, nil
}

func (c *Client) do(ctx context.Context, reqBody chatRequest) (string, error) {
	token, err := c.resolveToken(ctx)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("copilot: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("copilot: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("copilot: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("copilot: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("copilot: status %d: %s", resp.StatusCode, respBody)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("copilot: unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("copilot: empty response")
	}

	return chatResp.Choices[0].Message.Content, nil
}
