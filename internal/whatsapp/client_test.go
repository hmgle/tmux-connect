package whatsapp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
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

func TestParseMessageEventRejectsSelfChatByDefault(t *testing.T) {
	t.Parallel()

	client := &Client{}
	evt := testMessageEvent("wamid.in.1", "8613800000000", true, true)

	if _, ok := client.parseMessageEvent(evt); ok {
		t.Fatal("parseMessageEvent() accepted self-chat without allowSelfChat")
	}
}

func TestParseMessageEventAcceptsLinkedDeviceSelfChatWhenEnabled(t *testing.T) {
	t.Parallel()

	client := &Client{allowSelfChat: true}
	evt := testMessageEvent("wamid.in.1", "8613800000000", true, true)

	msg, ok := client.parseMessageEvent(evt)
	if !ok {
		t.Fatal("parseMessageEvent() rejected linked-device self-chat")
	}
	if !msg.IsFromMe {
		t.Fatal("msg.IsFromMe = false, want true")
	}
	if msg.ChatID != "8613800000000@s.whatsapp.net" || msg.SenderID != "8613800000000@s.whatsapp.net" {
		t.Fatalf("msg = %#v, want self-chat JIDs", msg)
	}
}

func TestParseMessageEventRejectsNonSelfDeviceSentMessage(t *testing.T) {
	t.Parallel()

	client := &Client{allowSelfChat: true}
	evt := testDirectDeviceSentEvent("wamid.in.1", "8613800000000", "8613900000000")

	if _, ok := client.parseMessageEvent(evt); ok {
		t.Fatal("parseMessageEvent() accepted non-self device-sent message")
	}
}

func TestParseMessageEventSuppressesTrackedOutboundMessage(t *testing.T) {
	t.Parallel()

	client := &Client{allowSelfChat: true}
	client.trackOutboundMessage("wamid.out.1")
	evt := testMessageEvent("wamid.out.1", "8613800000000", true, true)

	if _, ok := client.parseMessageEvent(evt); ok {
		t.Fatal("parseMessageEvent() accepted tracked outbound message")
	}
}

func testMessageEvent(messageID string, phone string, isFromMe bool, includeDeviceSentMeta bool) *events.Message {
	jid := waTypes.NewJID(phone, waTypes.DefaultUserServer)
	info := waTypes.MessageInfo{
		MessageSource: waTypes.MessageSource{
			Chat:     jid,
			Sender:   jid,
			IsFromMe: isFromMe,
		},
		ID:        waTypes.MessageID(messageID),
		Timestamp: time.Unix(10, 0),
	}
	if includeDeviceSentMeta {
		info.DeviceSentMeta = &waTypes.DeviceSentMeta{DestinationJID: jid.String()}
	}
	return &events.Message{
		Info: info,
		Message: &waE2E.Message{
			Conversation: proto.String("/panes"),
		},
	}
}

func testDirectDeviceSentEvent(messageID string, senderPhone string, chatPhone string) *events.Message {
	senderJID := waTypes.NewJID(senderPhone, waTypes.DefaultUserServer)
	chatJID := waTypes.NewJID(chatPhone, waTypes.DefaultUserServer)
	return &events.Message{
		Info: waTypes.MessageInfo{
			MessageSource: waTypes.MessageSource{
				Chat:     chatJID,
				Sender:   senderJID,
				IsFromMe: true,
			},
			ID:             waTypes.MessageID(messageID),
			Timestamp:      time.Unix(10, 0),
			DeviceSentMeta: &waTypes.DeviceSentMeta{DestinationJID: chatJID.String()},
		},
		Message: &waE2E.Message{
			Conversation: proto.String("/panes"),
		},
	}
}
