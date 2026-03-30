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
	BotClient
	openclaw *openclaw.Client

	mu       sync.Mutex
	sessions map[string]string // userID -> sessionKey
}

func NewChatMiddleware(bot *wechatbot.Bot, openclawClient *openclaw.Client) *ChatMiddleware {
	return &ChatMiddleware{
		BotClient: BotClient{Bot: bot},
		openclaw:  openclawClient,
		sessions:  make(map[string]string),
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

	sessionKey := m.getSessionKey(msg.UserID)
	response, err := m.openclaw.Chat(ctx, sessionKey, text)
	if err != nil && strings.Contains(err.Error(), "websocket: close sent") {
		slog.Warn("websocket closed, reconnecting", "user_id", msg.UserID)
		if rerr := m.openclaw.Reconnect(ctx); rerr != nil {
			slog.Error("openclaw reconnect failed", "user_id", msg.UserID, "error", rerr)
			m.Reply(ctx, msg, "处理消息失败，请稍后重试")
			return true
		}
		response, err = m.openclaw.Chat(ctx, sessionKey, text)
	}
	if err != nil {
		slog.Error("openclaw chat failed", "user_id", msg.UserID, "error", err)
		m.Reply(ctx, msg, "处理消息失败，请稍后重试")
		return true
	}

	response = strings.TrimSpace(response)
	if response == "" {
		response = "我暂时没有可用的回复。"
	}

	m.ReplyChunks(ctx, msg, response)
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
