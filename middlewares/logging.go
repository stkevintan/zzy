package middlewares

import (
	"context"
	"log/slog"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type LoggingMiddleware struct{}

var _ Middleware = (*LoggingMiddleware)(nil)

func (m *LoggingMiddleware) Name() string {
	return "logging"
}

func (m *LoggingMiddleware) HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool {
	slog.Info("Received message",
		"user_id", msg.UserID,
		"text", msg.Text,
		"type", msg.Type,
		"images", len(msg.Images),
		"voices", len(msg.Voices),
		"files", len(msg.Files),
	)
	return false
}
