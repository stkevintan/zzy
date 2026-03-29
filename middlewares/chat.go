package middlewares

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"zzy/copilot"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

const defaultChatSystemPrompt = `你是一个微信聊天助手。回答要直接、准确、简洁。除非用户要求，否则不要使用复杂格式。`

type ChatMiddleware struct {
	bot     *wechatbot.Bot
	copilot *copilot.Client

	mu       sync.Mutex
	sessions map[string][]copilot.Message
}

func NewChatMiddleware(bot *wechatbot.Bot, copilotClient *copilot.Client) *ChatMiddleware {
	return &ChatMiddleware{
		bot:      bot,
		copilot:  copilotClient,
		sessions: make(map[string][]copilot.Message),
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
		m.resetSession(msg.UserID)
		m.reply(ctx, msg, "已开始新的对话")
		return true
	}

	if strings.HasPrefix(text, "/") {
		return false
	}

	messages := m.buildConversation(msg.UserID, text)
	response, err := m.copilot.Chat(ctx, messages)
	if err != nil {
		slog.Error("copilot chat failed", "user_id", msg.UserID, "error", err)
		m.reply(ctx, msg, "处理消息失败，请稍后重试")
		return true
	}

	response = strings.TrimSpace(response)
	if response == "" {
		response = "我暂时没有可用的回复。"
	}

	m.commitConversation(msg.UserID, text, response)
	m.reply(ctx, msg, response)
	return true
}

func (m *ChatMiddleware) buildConversation(userID, text string) []copilot.Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	history := m.sessions[userID]
	if len(history) == 0 {
		history = []copilot.Message{{Role: "system", Content: defaultChatSystemPrompt}}
	}

	messages := append([]copilot.Message(nil), history...)
	messages = append(messages, copilot.Message{Role: "user", Content: text})
	return messages
}

func (m *ChatMiddleware) commitConversation(userID, userText, assistantText string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	history := m.sessions[userID]
	if len(history) == 0 {
		history = []copilot.Message{{Role: "system", Content: defaultChatSystemPrompt}}
	}
	history = append(history,
		copilot.Message{Role: "user", Content: userText},
		copilot.Message{Role: "assistant", Content: assistantText},
	)
	m.sessions[userID] = history
}

func (m *ChatMiddleware) resetSession(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, userID)
}

func (m *ChatMiddleware) reply(ctx context.Context, msg *wechatbot.IncomingMessage, text string) {
	if err := m.bot.Reply(ctx, msg, text); err != nil {
		slog.Error("failed to reply in chat middleware", "user_id", msg.UserID, "error", err)
	}
}
