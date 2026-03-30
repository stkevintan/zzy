package middlewares

import (
	"context"
	"log/slog"
	"strings"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type Middleware interface {
	Name() string
	HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool
}

// GetPriority returns the priority of a middleware.
// Smaller values run first. Default is 1.
func GetPriority(m Middleware) int {
	if p, ok := m.(interface{ Priority() int }); ok {
		return p.Priority()
	}
	return 1
}

// BotClient provides common reply helpers for middlewares.
type BotClient struct {
	Bot *wechatbot.Bot
}

// Reply sends a single message.
func (b *BotClient) Reply(ctx context.Context, msg *wechatbot.IncomingMessage, text string) {
	if err := b.Bot.Reply(ctx, msg, text); err != nil {
		slog.Error("failed to reply", "user_id", msg.UserID, "error", err)
	}
}

// ReplyChunks splits long text at newline boundaries and sends multiple messages.
func (b *BotClient) ReplyChunks(ctx context.Context, msg *wechatbot.IncomingMessage, text string) {
	const maxChunk = 1500
	for len(text) > maxChunk {
		splitAt := strings.LastIndex(text[:maxChunk], "\n")
		if splitAt <= 0 {
			splitAt = maxChunk
		}
		b.Reply(ctx, msg, text[:splitAt])
		text = strings.TrimSpace(text[splitAt:])
	}
	if text != "" {
		b.Reply(ctx, msg, text)
	}
}
