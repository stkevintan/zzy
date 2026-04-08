package middlewares

import (
	"context"
	"encoding/json"
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

func (m *ChatMiddleware) Priority() int {
	return 100
}

func (m *ChatMiddleware) HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool {
	var text string
	var attachment json.RawMessage
	switch msg.Type {
	case wechatbot.ContentText:
		text = msg.Text
	case wechatbot.ContentVoice:
		sb := strings.Builder{}
		for _, voice := range msg.Voices {
			sb.WriteString(voice.Text)
		}
		text = sb.String()
	case wechatbot.ContentImage:
		fallthrough
	case wechatbot.ContentVideo:
		fallthrough
	case wechatbot.ContentFile:
		media, err := m.Bot.Download(ctx, msg)
		if err != nil {
			slog.Error("failed to download media", "user_id", msg.UserID, "error", err)
			return false
		}
		attachment = media.Data
	default:
		return false
	}

	text = strings.TrimSpace(text)
	if text == "" && attachment == nil {
		return false
	}

	sessionKey := m.getSessionKey(msg.UserID)
	response, err := m.openclaw.Chat(ctx, sessionKey, text, attachment)
	if err != nil && strings.Contains(err.Error(), "websocket: close sent") {
		slog.Warn("websocket closed, reconnecting", "user_id", msg.UserID)
		if rerr := m.openclaw.Reconnect(ctx); rerr != nil {
			slog.Error("openclaw reconnect failed", "user_id", msg.UserID, "error", rerr)
			m.Reply(ctx, msg, "处理消息失败，请稍后重试")
			return true
		}
		response, err = m.openclaw.Chat(ctx, sessionKey, text, attachment)
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
