package botmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zzy/middlewares"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type BotMgrMiddleware struct {
	middlewares.BotClient
	manager *Manager
}

func NewMiddleware(manager *Manager, bot *wechatbot.Bot) *BotMgrMiddleware {
	return &BotMgrMiddleware{
		BotClient: middlewares.BotClient{Bot: bot},
		manager:   manager,
	}
}

var _ middlewares.Middleware = (*BotMgrMiddleware)(nil)

func (m *BotMgrMiddleware) OnStart(ctx context.Context) {
	entries, err := os.ReadDir(m.manager.credBaseDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// skip bots that already exist (e.g. master)
		if _, err := m.manager.GetBot(name); err == nil {
			continue
		}
		runtime, err := m.manager.CreateBot(name, false)
		if err != nil {
			continue
		}
		// auto login & start if credentials file exists
		credPath := filepath.Join(m.manager.credBaseDir, name, "credentials.json")
		if _, err := os.Stat(credPath); err == nil {
			if err := m.manager.LoginAndStartAsync(name); err != nil {
				runtime.logf("warn", "auto login failed: %v", err)
			}
		}
	}
}

func (m *BotMgrMiddleware) Name() string {
	return "botmgr"
}

func (m *BotMgrMiddleware) HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool {
	if msg.Type != wechatbot.ContentText {
		return false
	}

	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/bot") {
		return false
	}

	fields := strings.Fields(text)
	if len(fields) < 2 {
		m.Reply(ctx, msg, botUsage())
		return true
	}

	switch fields[1] {
	case "new":
		if len(fields) != 3 {
			m.Reply(ctx, msg, "用法: /bot new <name>")
			return true
		}
		if _, err := m.manager.CreateBot(fields[2], false); err != nil {
			m.Reply(ctx, msg, fmt.Sprintf("创建 bot 失败: %v", err))
			return true
		}
		m.Reply(ctx, msg, fmt.Sprintf("bot %s 已创建", fields[2]))
		return true
	case "del":
		if len(fields) != 3 {
			m.Reply(ctx, msg, "用法: /bot del <name>")
			return true
		}
		if err := m.manager.DeleteBot(fields[2]); err != nil {
			m.Reply(ctx, msg, fmt.Sprintf("删除 bot 失败: %v", err))
			return true
		}
		m.Reply(ctx, msg, fmt.Sprintf("bot %s 已删除", fields[2]))
		return true
	case "list":
		infos := m.manager.ListBots()
		lines := make([]string, 0, len(infos)+1)
		lines = append(lines, "当前 bots:")
		for _, info := range infos {
			status := "not logged in"
			if info.LoggedIn {
				status = "logged in"
			}
			if info.LoginInProgress {
				status += ", login in progress"
			}
			if info.Running {
				status += ", running"
			}
			prefix := "-"
			if info.IsMaster {
				prefix = "*"
			}
			lines = append(lines, fmt.Sprintf("%s %s: %s", prefix, info.Name, status))
		}
		m.ReplyChunks(ctx, msg, strings.Join(lines, "\n"))
		return true
	case "login":
		if len(fields) != 3 {
			m.Reply(ctx, msg, "用法: /bot login <name>")
			return true
		}
		if err := m.manager.LoginAndStartAsync(fields[2]); err != nil {
			m.Reply(ctx, msg, fmt.Sprintf("bot 登录失败: %v", err))
			return true
		}
		m.Reply(ctx, msg, fmt.Sprintf("bot %s 开始登录，请使用 /bot log %s 查看二维码和状态", fields[2], fields[2]))
		return true
	case "log":
		if len(fields) != 3 {
			m.Reply(ctx, msg, "用法: /bot log <name>")
			return true
		}
		lines, err := m.manager.LastLogLines(fields[2], 50)
		if err != nil {
			m.Reply(ctx, msg, fmt.Sprintf("读取 bot 日志失败: %v", err))
			return true
		}
		if len(lines) == 0 {
			m.Reply(ctx, msg, fmt.Sprintf("bot %s 暂无日志", fields[2]))
			return true
		}
		m.ReplyChunks(ctx, msg, strings.Join(lines, "\n"))
		return true
	default:
		m.Reply(ctx, msg, botUsage())
		return true
	}
}

func botUsage() string {
	return strings.Join([]string{
		"支持的命令:",
		"/bot new <name>",
		"/bot del <name>",
		"/bot list",
		"/bot login <name>",
		"/bot log <name>",
	}, "\n")
}
