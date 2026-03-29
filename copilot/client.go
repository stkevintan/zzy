package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	DefaultEndpoint = "https://api.githubcopilot.com/chat/completions"
	DefaultModel    = "gpt-4o"
)

// Client is a generic client for OpenAI-compatible chat completions APIs.
type Client struct {
	token    string
	endpoint string
	model    string
	client   *http.Client
}

// Option configures the Client.
type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) { c.endpoint = endpoint }
}

func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.client = hc }
}

// NewClient creates a new Copilot API client.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		token:    token,
		endpoint: DefaultEndpoint,
		model:    DefaultModel,
		client:   http.DefaultClient,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
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

	respMsg, err := c.do(ctx, reqBody)
	if err != nil {
		return "", err
	}
	return respMsg, nil
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
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("copilot: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("copilot: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
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
