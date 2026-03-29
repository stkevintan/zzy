package botmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"zzy/middlewares"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type MiddlewareFactory func(bot *wechatbot.Bot, locker *middlewares.Locker) []middlewares.Middleware

type BotInfo struct {
	Name            string
	IsMaster        bool
	LoggedIn        bool
	Running         bool
	LoginInProgress bool
}

type Manager struct {
	ctx         context.Context
	logLevel    string
	credBaseDir string
	factory     MiddlewareFactory

	mu         sync.RWMutex
	bots       map[string]*Runtime
	masterName string
}

type Runtime struct {
	manager  *Manager
	name     string
	isMaster bool

	bot      *wechatbot.Bot
	locker   *middlewares.Locker
	credPath string
	logs     *logBuffer

	mu              sync.Mutex
	middlewares     []middlewares.Middleware
	loggedIn        bool
	running         bool
	loginInProgress bool
	loginCancel     context.CancelFunc
}

func NewManager(ctx context.Context, logLevel, credBaseDir string, factory MiddlewareFactory) *Manager {
	return &Manager{
		ctx:         ctx,
		logLevel:    logLevel,
		credBaseDir: credBaseDir,
		factory:     factory,
		bots:        make(map[string]*Runtime),
	}
}

func (m *Manager) CreateBot(name string, isMaster bool) (*Runtime, error) {
	name = strings.TrimSpace(name)
	if err := validateBotName(name); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.bots[name]; exists {
		return nil, fmt.Errorf("bot %q already exists", name)
	}
	if isMaster {
		if m.masterName != "" {
			return nil, fmt.Errorf("master bot already exists")
		}
		m.masterName = name
	}

	credDir := filepath.Join(m.credBaseDir, name)
	if err := os.MkdirAll(credDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bot credential dir: %w", err)
	}

	runtime := &Runtime{
		manager:  m,
		name:     name,
		isMaster: isMaster,
		locker:   middlewares.NewLocker(),
		credPath: filepath.Join(credDir, "credentials.json"),
		logs:     newLogBuffer(name, 200),
	}

	runtime.bot = wechatbot.New(wechatbot.Options{
		CredPath: runtime.credPath,
		LogLevel: m.logLevel,
		OnQRURL: func(url string) {
			runtime.logf("info", "QR URL: %s", url)
		},
		OnScanned: func() {
			runtime.logf("info", "QR code scanned")
		},
		OnExpired: func() {
			runtime.logf("warn", "QR code expired")
		},
		OnError: func(err error) {
			runtime.logf("error", "SDK error: %v", err)
		},
	})
	runtime.middlewares = m.factory(runtime.bot, runtime.locker)
	runtime.bot.OnMessage(runtime.handleMessage)
	runtime.logf("info", "bot created (cred_path=%s)", runtime.credPath)

	m.bots[name] = runtime
	return runtime, nil
}

func (m *Manager) DeleteBot(name string) error {
	runtime, err := m.GetBot(name)
	if err != nil {
		return err
	}
	if runtime.isMaster {
		return fmt.Errorf("master bot cannot be deleted")
	}

	runtime.cancelLogin()
	runtime.Stop()

	m.mu.Lock()
	delete(m.bots, name)
	m.mu.Unlock()

	if err := os.RemoveAll(filepath.Dir(runtime.credPath)); err != nil {
		return fmt.Errorf("remove bot credential dir: %w", err)
	}
	return nil
}

func (m *Manager) GetBot(name string) (*Runtime, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	runtime, ok := m.bots[name]
	if !ok {
		return nil, fmt.Errorf("bot %q not found", name)
	}
	return runtime, nil
}

func (m *Manager) ListBots() []BotInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]BotInfo, 0, len(m.bots))
	for _, runtime := range m.bots {
		infos = append(infos, runtime.info())
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].IsMaster != infos[j].IsMaster {
			return infos[i].IsMaster
		}
		return infos[i].Name < infos[j].Name
	})
	return infos
}

func (m *Manager) LoginAndStartAsync(name string) error {
	runtime, err := m.GetBot(name)
	if err != nil {
		return err
	}
	if err := runtime.startLogin(); err != nil {
		return err
	}

	go runtime.loginAndStart()
	return nil
}

func (m *Manager) LastLogLines(name string, n int) ([]string, error) {
	runtime, err := m.GetBot(name)
	if err != nil {
		return nil, err
	}
	return runtime.logs.last(n), nil
}

func (r *Runtime) AddMiddleware(middleware middlewares.Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, middleware)
}

func (r *Runtime) Bot() *wechatbot.Bot {
	return r.bot
}

func (r *Runtime) Login(ctx context.Context, force bool) (*wechatbot.Credentials, error) {
	if err := r.startLogin(); err != nil {
		return nil, err
	}
	return r.login(ctx, force)
}

func (r *Runtime) Start(ctx context.Context) error {
	if err := r.markRunning(); err != nil {
		return err
	}
	defer r.markStopped()

	r.logf("info", "poll loop starting")
	err := r.bot.Run(ctx)
	if err != nil {
		r.logf("error", "poll loop stopped with error: %v", err)
		return err
	}
	r.logf("info", "poll loop stopped")
	return nil
}

func (r *Runtime) StartAsync() error {
	if err := r.markRunning(); err != nil {
		return err
	}

	go func() {
		defer r.markStopped()
		r.logf("info", "poll loop starting")
		if err := r.bot.Run(r.manager.ctx); err != nil {
			r.logf("error", "poll loop stopped with error: %v", err)
			return
		}
		r.logf("info", "poll loop stopped")
	}()
	return nil
}

func (r *Runtime) Stop() {
	r.bot.Stop()
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
	r.logf("info", "bot stop requested")
}

func (r *Runtime) info() BotInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return BotInfo{
		Name:            r.name,
		IsMaster:        r.isMaster,
		LoggedIn:        r.loggedIn,
		Running:         r.running,
		LoginInProgress: r.loginInProgress,
	}
}

func (r *Runtime) handleMessage(msg *wechatbot.IncomingMessage) {
	ctx := r.manager.ctx
	if err := r.bot.SendTyping(ctx, msg.UserID); err != nil {
		r.logf("warn", "failed to send typing indicator: %v", err)
	}
	defer func() {
		if err := r.bot.StopTyping(ctx, msg.UserID); err != nil {
			r.logf("warn", "failed to stop typing indicator: %v", err)
		}
	}()

	middlewares := r.snapshotMiddlewares()
	for _, middleware := range middlewares {
		if r.locker.IsLockedByOther(middleware.Name()) {
			continue
		}
		if middleware.HandleMessage(ctx, msg) {
			return
		}
	}
}

func (r *Runtime) loginAndStart() {
	if _, err := r.login(r.manager.ctx, false); err != nil {
		return
	}
	if err := r.StartAsync(); err != nil {
		r.logf("warn", "failed to start poll loop after login: %v", err)
	}
}

func (r *Runtime) login(ctx context.Context, force bool) (*wechatbot.Credentials, error) {
	loginCtx, cancel := context.WithCancel(ctx)
	r.setLoginCancel(cancel)
	defer cancel()

	r.logf("info", "login started")
	creds, err := r.bot.Login(loginCtx, force)
	if err != nil {
		r.finishLogin(false)
		r.logf("error", "login failed: %v", err)
		return nil, err
	}

	r.finishLogin(true)
	r.logf("info", "login completed (account_id=%s)", creds.AccountID)
	return creds, nil
}

func (r *Runtime) startLogin() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("bot %q is already running", r.name)
	}
	if r.loginInProgress {
		return fmt.Errorf("bot %q is already logging in", r.name)
	}
	r.loginInProgress = true
	return nil
}

func (r *Runtime) finishLogin(loggedIn bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loggedIn = loggedIn
	r.loginInProgress = false
	r.loginCancel = nil
}

func (r *Runtime) setLoginCancel(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loginCancel = cancel
}

func (r *Runtime) cancelLogin() {
	r.mu.Lock()
	cancel := r.loginCancel
	r.loginCancel = nil
	r.loginInProgress = false
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		r.logf("info", "login cancelled")
	}
}

func (r *Runtime) markRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("bot %q is already running", r.name)
	}
	r.running = true
	return nil
}

func (r *Runtime) markStopped() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
}

func (r *Runtime) snapshotMiddlewares() []middlewares.Middleware {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]middlewares.Middleware(nil), r.middlewares...)
}

func (r *Runtime) logf(level, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	r.logs.add(level, line)
}

func validateBotName(name string) error {
	if name == "" {
		return fmt.Errorf("bot name cannot be empty")
	}
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			continue
		}
		return fmt.Errorf("bot name %q contains unsupported character %q", name, ch)
	}
	return nil
}

type logBuffer struct {
	name  string
	mu    sync.Mutex
	limit int
	lines []string
}

func newLogBuffer(name string, limit int) *logBuffer {
	return &logBuffer{name: name, limit: limit}
}

func (b *logBuffer) add(level, line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := fmt.Sprintf("%s [%s] [%s] %s", time.Now().Format("2006-01-02 15:04:05"), strings.ToUpper(level), b.name, line)
	b.lines = append(b.lines, entry)
	if len(b.lines) > b.limit {
		b.lines = append([]string(nil), b.lines[len(b.lines)-b.limit:]...)
	}
	_, _ = fmt.Fprintln(logWriter(level), entry)
}

func logWriter(level string) *os.File {
	switch strings.ToLower(level) {
	case "warn", "warning", "error":
		return os.Stderr
	default:
		return os.Stdout
	}
}

func (b *logBuffer) last(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || n >= len(b.lines) {
		return append([]string(nil), b.lines...)
	}
	return append([]string(nil), b.lines[len(b.lines)-n:]...)
}
