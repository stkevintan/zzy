package middlewares

import (
	"context"

	wechatbot "github.com/corespeed-io/wechatbot/golang"
)

type Middleware interface {
	HandleMessage(ctx context.Context, msg *wechatbot.IncomingMessage) bool
}
