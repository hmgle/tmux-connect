package daemon

import (
	"context"
	"testing"

	"github.com/hmgle/tmux-connect/internal/feishu"
)

type fakeFeishuClient struct {
	lastChatID string
	lastText   string
	lastCard   string
	lastImage  []byte
	lastOpts   feishu.SendOptions
	runEvent   feishu.MessageEvent
}

func (f *fakeFeishuClient) Run(ctx context.Context, handler func(context.Context, feishu.MessageEvent) error) error {
	if f.runEvent.ChatID == "" {
		return nil
	}
	return handler(ctx, f.runEvent)
}

func (f *fakeFeishuClient) SendText(_ context.Context, chatID string, text string, opts feishu.SendOptions) (string, error) {
	f.lastChatID = chatID
	f.lastText = text
	f.lastOpts = opts
	return "om_text", nil
}

func (f *fakeFeishuClient) SendCard(_ context.Context, chatID string, card string, opts feishu.SendOptions) (string, error) {
	f.lastChatID = chatID
	f.lastCard = card
	f.lastOpts = opts
	return "om_card", nil
}

func (f *fakeFeishuClient) SendImage(_ context.Context, chatID string, data []byte, opts feishu.SendOptions) (string, error) {
	f.lastChatID = chatID
	f.lastImage = append([]byte(nil), data...)
	f.lastOpts = opts
	return "om_image", nil
}

func TestFeishuAdapterSendMessageUsesCardWhenProvided(t *testing.T) {
	t.Parallel()

	client := &fakeFeishuClient{}
	adapter := &feishuAdapter{client: client}

	message, err := adapter.SendMessage(context.Background(), feishuChat("oc_chat_1"), "ignored", SendOptions{
		ReplyToMessageID: "om_parent",
		ThreadID:         "omt_thread",
		Card:             `{"header":{"title":{"tag":"plain_text","content":"Help"}}}`,
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.MessageID != "om_card" {
		t.Fatalf("message id = %q, want om_card", message.MessageID)
	}
	if client.lastCard == "" || client.lastText != "" {
		t.Fatalf("client state = %#v, want card send", client)
	}
	if client.lastOpts.ReplyToMessageID != "om_parent" || client.lastOpts.ThreadID != "omt_thread" {
		t.Fatalf("send opts = %#v", client.lastOpts)
	}
}

func TestFeishuAdapterRunMapsInboundMessage(t *testing.T) {
	t.Parallel()

	client := &fakeFeishuClient{
		runEvent: feishu.MessageEvent{
			ChatID:       "oc_chat_1",
			MessageID:    "om_1",
			UserID:       "ou_1",
			Text:         "/panes",
			ChatType:     "group",
			ThreadID:     "omt_1",
			IsAppMention: true,
		},
	}
	adapter := &feishuAdapter{client: client}
	var got IncomingMessage
	err := adapter.Run(context.Background(), func(_ context.Context, message IncomingMessage) error {
		got = message
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got.Chat.Platform != "feishu" || got.Chat.ChatID != "oc_chat_1" {
		t.Fatalf("chat = %#v, want feishu/oc_chat_1", got.Chat)
	}
	if got.PendingScope != "omt_1" || got.ThreadID != "omt_1" {
		t.Fatalf("thread mapping = %#v", got)
	}
	if !got.IsAppMention {
		t.Fatal("IsAppMention = false, want true")
	}
}

func TestFeishuAdapterPromptOptionsReplyInThread(t *testing.T) {
	t.Parallel()

	adapter := &feishuAdapter{}
	opts := adapter.PromptOptions(IncomingMessage{
		MessageID: "om_parent",
		ThreadID:  "omt_thread",
	}, commandPromptSpec{})
	if opts.ReplyToMessageID != "om_parent" || opts.ThreadID != "omt_thread" {
		t.Fatalf("PromptOptions() = %#v, want reply+thread context", opts)
	}
}
