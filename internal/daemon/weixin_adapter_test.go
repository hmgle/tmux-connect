//go:build !no_weixin

package daemon

import (
	"context"
	"io"
	"testing"

	wx "github.com/hmgle/tmux-connect/internal/weixin"
)

type fakeWeixinClient struct {
	runEvents  []wx.MessageEvent
	textCalls  []weixinTextCall
	imageCalls []weixinImageCall
}

type weixinTextCall struct {
	chatID string
	text   string
}

type weixinImageCall struct {
	chatID   string
	fileName string
	caption  string
}

func (f *fakeWeixinClient) Run(ctx context.Context, handler func(context.Context, wx.MessageEvent) error) error {
	for _, event := range f.runEvents {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return nil
}

func (f *fakeWeixinClient) SendText(_ context.Context, chatID string, text string) (string, error) {
	f.textCalls = append(f.textCalls, weixinTextCall{chatID: chatID, text: text})
	return "wx-out-1", nil
}

func (f *fakeWeixinClient) SendImage(_ context.Context, chatID string, fileName string, data []byte, caption string) (string, error) {
	f.imageCalls = append(f.imageCalls, weixinImageCall{chatID: chatID, fileName: fileName, caption: caption})
	return "wx-img-1", nil
}

func (f *fakeWeixinClient) Close() error { return nil }

func TestWeixinAdapterSendMessage(t *testing.T) {
	t.Parallel()

	client := &fakeWeixinClient{}
	adapter := &weixinAdapter{client: client, stderr: io.Discard}

	_, err := adapter.SendMessage(context.Background(), ChatRef{Platform: "weixin", ChatID: "user@im.wechat"}, "hello", SendOptions{})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(client.textCalls) != 1 || client.textCalls[0].chatID != "user@im.wechat" || client.textCalls[0].text != "hello" {
		t.Fatalf("textCalls = %#v", client.textCalls)
	}
}

func TestWeixinAdapterRunMapsInboundMessage(t *testing.T) {
	t.Parallel()

	client := &fakeWeixinClient{
		runEvents: []wx.MessageEvent{{
			ChatID:    "user@im.wechat",
			SenderID:  "user@im.wechat",
			MessageID: "wx-in-1",
			Text:      "/panes",
		}},
	}
	adapter := &weixinAdapter{client: client, stderr: io.Discard}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan IncomingMessage, 1)
	go func() {
		_ = adapter.Run(ctx, func(_ context.Context, message IncomingMessage) error {
			done <- message
			cancel()
			return nil
		})
	}()

	got := <-done
	if got.Chat.Platform != "weixin" || got.Chat.ChatID != "user@im.wechat" {
		t.Fatalf("chat = %#v", got.Chat)
	}
	if got.UserID != "user@im.wechat" || got.Text != "/panes" || got.ChatType != "private" {
		t.Fatalf("incoming message = %#v", got)
	}
}
