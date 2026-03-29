package middlewares

import (
	"context"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type Middleware interface {
	Name() string
	HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool
}
