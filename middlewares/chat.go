package middlewares

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"zzy/openclaw"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type ChatMiddleware struct {
	bot      *wechatbot.Bot
	openclaw *openclaw.Client

	mu       sync.Mutex
	sessions map[string]string // userID -> sessionKey
}

func NewChatMiddleware(bot *wechatbot.Bot, openclawClient *openclaw.Client) *ChatMiddleware {
	return &ChatMiddleware{
		bot:      bot,
		openclaw: openclawClient,
		sessions: make(map[string]string),
	}
}

var _ Middleware = (*ChatMiddleware)(nil)

func (m *ChatMiddleware) Name() string {
	return "chat"
}

func (m *ChatMiddleware) HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool {
	if msg.Type != wechatbot.ContentText {
		return false
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return false
	}

	if text == "/new" {
		m.resetSession(ctx, msg)
		return true
	}

	if strings.HasPrefix(text, "/") {
		return false
	}

	sessionKey := m.getSessionKey(msg.UserID)
	response, err := m.openclaw.Chat(ctx, sessionKey, text)
	if err != nil {
		slog.Error("openclaw chat failed", "user_id", msg.UserID, "error", err)
		m.reply(ctx, msg, "处理消息失败，请稍后重试")
		return true
	}

	response = strings.TrimSpace(response)
	if response == "" {
		response = "我暂时没有可用的回复。"
	}

	m.reply(ctx, msg, response)
	return true
}

func (m *ChatMiddleware) getSessionKey(userID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, ok := m.sessions[userID]
	if !ok {
		key = fmt.Sprintf("wechat-%s", userID)
		m.sessions[userID] = key
	}
	return key
}

func (m *ChatMiddleware) resetSession(ctx context.Context, msg *wechatbot.IncomingMessage) {
	sessionKey := m.getSessionKey(msg.UserID)
	if err := m.openclaw.ResetSession(ctx, sessionKey); err != nil {
		slog.Warn("failed to reset openclaw session", "user_id", msg.UserID, "error", err)
	}
	m.reply(ctx, msg, "已开始新的对话")
}

func (m *ChatMiddleware) reply(ctx context.Context, msg *wechatbot.IncomingMessage, text string) {
	if err := m.bot.Reply(ctx, msg, text); err != nil {
		slog.Error("failed to reply in chat middleware", "user_id", msg.UserID, "error", err)
	}
}
