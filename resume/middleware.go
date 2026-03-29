package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"zzy/copilot"
	"zzy/middlewares"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

const resumeParsePrompt = `你是一个简历解析助手。请从提供的简历文本中提取以下信息，并以JSON格式返回。

字段说明：
- "学科": 应聘学科
- "姓名": 姓名
- "性别": 只能是 "男" 或 "女"
- "身份证号": 身份证号码
- "年龄": 年龄（整数）
- "民族": 民族
- "籍贯": 籍贯
- "政治面貌": 只能是 "党员"、"预备党员"、"群众"、"团员" 之一
- "最高学历": 只能是 "本科"、"硕士"、"博士" 之一
- "手机号码": 手机号码
- "职称": 只能是 "无"、"二级教师"、"一级教师"、"高级"、"正高级" 之一
- "工作年月": 参加工作的年月
- "毕业院校_本科": 本科毕业院校
- "本科专业": 本科所学专业
- "毕业院校_硕士": 硕士毕业院校（如无则留空）
- "硕士专业": 硕士所学专业（如无则留空）
- "毕业时间": 毕业时间
- "教师资格证书类型": 只能是 "小学"、"初中"、"高中"、"中职" 之一
- "现工作单位": 当前工作单位
- "工作经验": 工作经历摘要
- "主要荣誉": 主要荣誉和奖项
- "预计税前月薪_万每月": 期望月薪（字符串）
- "预计税前薪资_万每年": 期望年薪（字符串）
- "备注": 其他备注信息

如果某个字段在简历中找不到，请将字符串字段设为空字符串""，整数字段设为0。所有字段的值类型必须严格匹配：年龄为整数，其余全部为字符串。只返回JSON，不要包含其他文字。`

var resumeExts = map[string]bool{
	".doc": true, ".docx": true, ".pdf": true,
}

type Result struct {
	FileName string      `json:"file_name"`
	FileType string      `json:"file_type"`
	Content  string      `json:"content"`
	Error    string      `json:"error,omitempty"`
	Entry    ResumeEntry `json:"entry,omitempty"`
}

type task struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	results []Result
	wg      sync.WaitGroup

	replyMsg *wechatbot.IncomingMessage
}

type Middleware struct {
	bot     *wechatbot.Bot
	copilot *copilot.Client
	locker  *middlewares.Locker

	mu   sync.Mutex
	task *task
}

func NewMiddleware(bot *wechatbot.Bot, copilotClient *copilot.Client, locker *middlewares.Locker) *Middleware {
	return &Middleware{bot: bot, copilot: copilotClient, locker: locker}
}

var _ middlewares.Middleware = (*Middleware)(nil)

func (m *Middleware) Name() string {
	return "resume"
}

func (m *Middleware) HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool {
	switch strings.TrimSpace(msg.Text) {
	case "/resume start":
		m.start(ctx, msg)
		return true
	case "/resume stop":
		m.stop(ctx, msg)
		return true
	}

	// Accept files only when a task is active
	if msg.Type == wechatbot.ContentFile {
		m.mu.Lock()
		t := m.task
		m.mu.Unlock()

		if t != nil {
			m.handleFile(ctx, t, msg)
			return true
		}
	}

	return false
}

func (m *Middleware) start(ctx context.Context, msg *wechatbot.IncomingMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel any existing task
	if m.task != nil {
		m.task.cancel()
	}

	taskCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	m.task = &task{
		ctx:      taskCtx,
		cancel:   cancel,
		replyMsg: msg,
	}
	m.locker.Lock(m.Name())

	m.reply(ctx, msg, "任务创建成功，请在10分钟内发送简历文件（支持 doc/docx/pdf）")

	// Watch for timeout in background
	t := m.task
	go func() {
		<-taskCtx.Done()
		if taskCtx.Err() == context.DeadlineExceeded {
			slog.Info("resume task timed out, finishing")
			m.finish(ctx, t)
		}
	}()
}

func (m *Middleware) stop(ctx context.Context, msg *wechatbot.IncomingMessage) {
	m.mu.Lock()
	t := m.task
	m.mu.Unlock()

	if t == nil {
		m.reply(ctx, msg, "当前没有进行中的简历任务")
		return
	}

	t.cancel()
	m.finish(ctx, t)
}

func (m *Middleware) handleFile(ctx context.Context, t *task, msg *wechatbot.IncomingMessage) {
	// Check context is still active
	if t.ctx.Err() != nil {
		return
	}

	for _, file := range msg.Files {
		ext := strings.ToLower(filepath.Ext(file.FileName))
		if !resumeExts[ext] {
			slog.Info("skipping non-resume file", "file", file.FileName, "ext", ext)
			m.reply(ctx, msg, fmt.Sprintf("跳过非简历文件: %s（仅支持 doc/docx/pdf）", file.FileName))
			continue
		}

		t.wg.Add(1)
		go func(f wechatbot.FileContent) {
			defer t.wg.Done()

			slog.Info("downloading resume", "file", f.FileName)
			data, err := m.bot.DownloadRaw(ctx, f.Media, "")
			if err != nil {
				slog.Error("download failed", "file", f.FileName, "error", err)
				t.addResult(Result{
					FileName: f.FileName,
					FileType: ext,
					Error:    fmt.Sprintf("download failed: %v", err),
				})
				return
			}

			slog.Info("extracting text", "file", f.FileName, "size", len(data))
			content, err := ExtractText(data, ext)
			if err != nil {
				slog.Error("extraction failed", "file", f.FileName, "error", err)
				t.addResult(Result{
					FileName: f.FileName,
					FileType: ext,
					Error:    fmt.Sprintf("extraction failed: %v", err),
				})
				return
			}

			slog.Info("parsing resume with copilot", "file", f.FileName)
			entry, err := copilot.Parse[ResumeEntry](ctx, m.copilot, resumeParsePrompt, content)
			if err != nil {
				slog.Error("copilot parse failed", "file", f.FileName, "error", err)
				t.addResult(Result{
					FileName: f.FileName,
					FileType: ext,
					Content:  content,
					Error:    fmt.Sprintf("parse failed: %v", err),
				})
				return
			}

			t.addResult(Result{
				FileName: f.FileName,
				FileType: ext,
				Content:  content,
				Entry:    *entry,
			})
			slog.Info("resume processed", "file", f.FileName, "content_len", len(content))
		}(file)

		m.reply(ctx, msg, fmt.Sprintf("正在处理: %s", file.FileName))
	}
}

func (m *Middleware) finish(ctx context.Context, t *task) {
	// Wait for all goroutines to complete
	t.wg.Wait()

	m.mu.Lock()
	if m.task == t {
		m.task = nil
	}
	m.mu.Unlock()
	m.locker.Unlock(m.Name())

	t.mu.Lock()
	results := t.results
	t.mu.Unlock()

	if len(results) == 0 {
		m.reply(ctx, t.replyMsg, "任务结束，未收到任何简历文件")
		return
	}

	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		m.reply(ctx, t.replyMsg, fmt.Sprintf("JSON序列化失败: %v", err))
		return
	}

	m.reply(ctx, t.replyMsg, fmt.Sprintf("共处理 %d 份简历", len(results)))
	m.replyContent(ctx, t.replyMsg, wechatbot.SendFile(jsonData, "resumes.json"))

	xlsxData, err := ExportXLSX(results, "")
	if err != nil {
		slog.Error("xlsx export failed", "error", err)
		m.reply(ctx, t.replyMsg, fmt.Sprintf("XLSX导出失败: %v", err))
		return
	}
	m.replyContent(ctx, t.replyMsg, wechatbot.SendFile(xlsxData, "简历汇总.xlsx"))
}

func (m *Middleware) reply(ctx context.Context, msg *wechatbot.IncomingMessage, text string) {
	if err := m.bot.Reply(ctx, msg, text); err != nil {
		slog.Error("failed to reply", "error", err, "text", text)
	}
}

func (m *Middleware) replyContent(ctx context.Context, msg *wechatbot.IncomingMessage, content wechatbot.SendContent) {
	if err := m.bot.ReplyContent(ctx, msg, content); err != nil {
		slog.Error("failed to reply with content", "error", err)
	}
}

func (t *task) addResult(r Result) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.results = append(t.results, r)
}
