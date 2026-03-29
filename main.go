package main

import (
	"context"
	"log/slog"
	"os"
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
	bot := wechatbot.New()

	copilotClient := copilot.NewClient(
		cfg.Copilot.Token,
		copilot.WithModel(cfg.Copilot.Model),
		copilot.WithEndpoint(cfg.Copilot.Endpoint),
	)

	creds, _ := bot.Login(ctx, false)
	slog.Info("Logged in", "account_id", creds.AccountID)
	middlewares := []middlewares.Middleware{
		&middlewares.LoggingMiddleware{},
		resume.NewMiddleware(bot, copilotClient),
	}

	bot.OnMessage(func(msg *wechatbot.IncomingMessage) {
		bot.SendTyping(ctx, creds.UserID)
		defer bot.StopTyping(ctx, creds.UserID)
		for _, m := range middlewares {
			if m.HandleMessage(ctx, msg) {
				return
			}
		}
		// // Download and save images
		// for i, img := range msg.Images {
		// 	fmt.Printf("Image[%d]: %dx%d, AESKey=%s\n", i, img.Width, img.Height, img.AESKey)

		// 	// Option 1: Download specific image (handles CDN fetch + AES-128-ECB decryption)
		// 	media, err := bot.DownloadRaw(ctx, img.Media, img.AESKey)
		// 	if err != nil {
		// 		fmt.Printf("  download error: %v\n", err)
		// 		continue
		// 	}

		// 	outPath := filepath.Join("images", fmt.Sprintf("%s_%d.jpg", msg.UserID, i))
		// 	os.MkdirAll(filepath.Dir(outPath), 0o755)
		// 	if err := os.WriteFile(outPath, media, 0o644); err != nil {
		// 		fmt.Printf("  save error: %v\n", err)
		// 		continue
		// 	}
		// 	fmt.Printf("  saved to %s (%d bytes)\n", outPath, len(media))
		// }
		// for _, voice := range msg.Voices {
		// 	fmt.Printf("Voice text: %s (%dms)\n", voice.Text, voice.DurationMs)
		// }
		// for i, file := range msg.Files {
		// 	fmt.Printf("File[%d]: %s (%d bytes)\n", i, file.FileName, file.Size)

		// 	media, err := bot.DownloadRaw(ctx, file.Media, "")
		// 	if err != nil {
		// 		fmt.Printf("  download error: %v\n", err)
		// 		continue
		// 	}

		// 	outPath := filepath.Join("files", file.FileName)
		// 	os.MkdirAll(filepath.Dir(outPath), 0o755)
		// 	if err := os.WriteFile(outPath, media, 0o644); err != nil {
		// 		fmt.Printf("  save error: %v\n", err)
		// 		continue
		// 	}
		// 	fmt.Printf("  saved to %s (%d bytes)\n", outPath, len(media))
		// }
		// if msg.QuotedMessage != nil {
		// 	fmt.Printf("Quoted: %s\n", msg.QuotedMessage.Title)
		// }
		// if msg.Type == "text" {
		// 	bot.Reply(ctx, msg, fmt.Sprintf("Echo %s", msg.Text))
		// 	bot.Reply(ctx, msg, "Done")
		// }
	})

	bot.Run(ctx)
}
