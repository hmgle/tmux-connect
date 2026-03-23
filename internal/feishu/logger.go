package feishu

import (
	"context"
	"io"
	"log"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type sdkLogger struct {
	logger *log.Logger
}

func newSDKLogger(w io.Writer) larkcore.Logger {
	if w == nil {
		w = io.Discard
	}
	return sdkLogger{logger: log.New(w, "feishu-sdk: ", log.LstdFlags)}
}

func (l sdkLogger) Debug(context.Context, ...interface{}) {}

func (l sdkLogger) Info(context.Context, ...interface{}) {}

func (l sdkLogger) Warn(_ context.Context, args ...interface{}) {
	l.logger.Print(args...)
}

func (l sdkLogger) Error(_ context.Context, args ...interface{}) {
	l.logger.Print(args...)
}
