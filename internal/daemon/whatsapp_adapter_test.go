package daemon

import (
	"context"
	"io"
	"testing"
	"time"

	wa "github.com/hmgle/tmux-connect/internal/whatsapp"
)

type fakeWhatsAppClient struct {
	autoMarkRead bool
	runEvents    []wa.MessageEvent
	textCalls    []whatsAppTextCall
	imageCalls   []whatsAppImageCall
}

type whatsAppTextCall struct {
	chatID           string
	text             string
	replyToMessageID string
	replyToSenderID  string
}

type whatsAppImageCall struct {
	chatID           string
	fileName         string
	caption          string
	replyToMessageID string
	replyToSenderID  string
}

func (f *fakeWhatsAppClient) Run(ctx context.Context, autoMarkRead bool, handler func(context.Context, wa.MessageEvent) error) error {
	f.autoMarkRead = autoMarkRead
	for _, event := range f.runEvents {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return nil
}

func (f *fakeWhatsAppClient) SendText(_ context.Context, chatID string, text string, replyToMessageID string, replyToSenderID string) (string, error) {
	f.textCalls = append(f.textCalls, whatsAppTextCall{
		chatID:           chatID,
		text:             text,
		replyToMessageID: replyToMessageID,
		replyToSenderID:  replyToSenderID,
	})
	return "wamid.out.1", nil
}

func (f *fakeWhatsAppClient) SendImage(_ context.Context, chatID string, fileName string, data []byte, caption string, replyToMessageID string, replyToSenderID string) (string, error) {
	f.imageCalls = append(f.imageCalls, whatsAppImageCall{
		chatID:           chatID,
		fileName:         fileName,
		caption:          caption,
		replyToMessageID: replyToMessageID,
		replyToSenderID:  replyToSenderID,
	})
	return "wamid.image.1", nil
}

func (f *fakeWhatsAppClient) Close() error { return nil }

func TestWhatsAppAdapterSendMessageUsesReplyMetadata(t *testing.T) {
	t.Parallel()

	client := &fakeWhatsAppClient{}
	adapter := &whatsappAdapter{client: client, stderr: io.Discard, autoMarkRead: true}

	_, err := adapter.SendMessage(context.Background(), ChatRef{Platform: "whatsapp", ChatID: "8613800000000@s.whatsapp.net"}, "hello", SendOptions{
		ReplyToMessageID: "wamid.in.1",
		ReplyToSenderID:  "8613800000000@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if len(client.textCalls) != 1 {
		t.Fatalf("textCalls = %#v, want one send", client.textCalls)
	}
	if client.textCalls[0].replyToMessageID != "wamid.in.1" || client.textCalls[0].replyToSenderID != "8613800000000@s.whatsapp.net" {
		t.Fatalf("text call = %#v, want reply metadata", client.textCalls[0])
	}
}

func TestWhatsAppAdapterRunMapsInboundMessage(t *testing.T) {
	t.Parallel()

	client := &fakeWhatsAppClient{
		runEvents: []wa.MessageEvent{{
			ChatID:          "8613800000000@s.whatsapp.net",
			SenderID:        "8613800000000@s.whatsapp.net",
			MessageID:       "wamid.in.1",
			Text:            "/panes",
			QuotedMessageID: "wamid.prompt.1",
			QuotedSenderID:  "12345@s.whatsapp.net",
			Timestamp:       time.Unix(10, 0),
		}},
	}
	adapter := &whatsappAdapter{client: client, stderr: io.Discard, autoMarkRead: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got IncomingMessage
	done := make(chan struct{})
	go func() {
		_ = adapter.Run(ctx, func(_ context.Context, message IncomingMessage) error {
			got = message
			cancel()
			close(done)
			return nil
		})
	}()
	<-done

	if !client.autoMarkRead {
		t.Fatal("autoMarkRead = false, want true")
	}
	if got.Chat.Platform != "whatsapp" || got.Chat.ChatID != "8613800000000@s.whatsapp.net" {
		t.Fatalf("chat = %#v, want whatsapp DM", got.Chat)
	}
	if got.UserID != "8613800000000@s.whatsapp.net" || got.IsFromSelf {
		t.Fatalf("incoming message = %#v, want inbound sender and non-self flag", got)
	}
	if got.QuotedMessageID != "wamid.prompt.1" || got.QuotedSenderID != "12345@s.whatsapp.net" {
		t.Fatalf("incoming message = %#v, want quoted metadata", got)
	}
}
