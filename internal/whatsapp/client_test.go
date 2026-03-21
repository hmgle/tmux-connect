package whatsapp

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestHandleMessageEventUsesParentContext(t *testing.T) {
	t.Parallel()

	client := &Client{stderr: io.Discard}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := make(chan struct{}, 1)
	client.handleMessageEvent(ctx, false, MessageEvent{}, func(runCtx context.Context, _ MessageEvent) error {
		if !errors.Is(runCtx.Err(), context.Canceled) {
			t.Fatalf("handler context err = %v, want %v", runCtx.Err(), context.Canceled)
		}
		called <- struct{}{}
		return nil
	})

	select {
	case <-called:
	default:
		t.Fatal("handler was not called")
	}
}
