package feishu

import (
	"errors"
	"strings"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestParseMessageEventStripsMentionsInGroups(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{OpenId: stringPtr("ou_sender")},
			},
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"<at user_id=\"ou_bot\">tmux-connect</at> /panes"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event)
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if message.Text != "/panes" {
		t.Fatalf("text = %q, want /panes", message.Text)
	}
	if !message.IsAppMention {
		t.Fatal("IsAppMention = false, want true")
	}
}

func TestParseMessageEventMentionOnlyDefaultsToHelp(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"<at user_id=\"ou_bot\">tmux-connect</at>"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event)
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if message.Text != "help" {
		t.Fatalf("text = %q, want help", message.Text)
	}
}

func TestFeishuAPIErrorIncludesCredentialHintForAuthFailures(t *testing.T) {
	t.Parallel()

	err := feishuAPIError("send message", 99991663, "invalid app_access_token")
	if err == nil {
		t.Fatal("feishuAPIError() = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "TMUXCONN_FEISHU_APP_ID") || !strings.Contains(got, "permissions") {
		t.Fatalf("error = %q, want credential hint", got)
	}
}

func TestWrapFeishuErrorIncludesCredentialHintForSDKFailures(t *testing.T) {
	t.Parallel()

	err := wrapFeishuError("start websocket client", errors.New("permission denied"))
	if err == nil {
		t.Fatal("wrapFeishuError() = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "Feishu app permissions") && !strings.Contains(got, "permissions") {
		t.Fatalf("error = %q, want permission hint", got)
	}
}

func stringPtr(value string) *string {
	return &value
}
