package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"zzy/botmgr"
	"zzy/config"
	"zzy/copilot"
	"zzy/middlewares"
	"zzy/resume"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.Log.SlogLevel(),
	})))

	ctx := context.Background()

	githubToken, err := copilot.Login()
	if err != nil {
		slog.Error("copilot login failed", "error", err)
		os.Exit(1)
	}

	copilotClient := copilot.NewClient(
		githubToken,
		copilot.WithModel(cfg.Copilot.Model),
	)

	manager := botmgr.NewManager(
		ctx,
		cfg.Log.Level,
		filepath.Join("data", "bots"),
		func(bot *wechatbot.Bot, locker *middlewares.Locker) []middlewares.Middleware {
			return []middlewares.Middleware{
				&middlewares.LoggingMiddleware{},
				resume.NewMiddleware(bot, copilotClient, locker),
				middlewares.NewChatMiddleware(bot, copilotClient),
			}
		},
	)

	masterBot, err := manager.CreateBot("master", true)
	if err != nil {
		slog.Error("failed to create master bot", "error", err)
		os.Exit(1)
	}
	masterBot.AddMiddleware(botmgr.NewMiddleware(manager, masterBot.Bot()))

	creds, err := masterBot.Login(ctx, false)
	if err != nil {
		slog.Error("master bot login failed", "error", err)
		os.Exit(1)
	}
	slog.Info("master bot logged in", "account_id", creds.AccountID)

	if err := masterBot.Start(ctx); err != nil {
		slog.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}
