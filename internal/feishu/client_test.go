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
				Content:     stringPtr(`{"text":"@_user_1 /panes"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
					Id:  &larkim.UserId{OpenId: stringPtr("ou_bot")},
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, botIdentitySet(BotIdentity{OpenID: "ou_bot"}))
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
				Content:     stringPtr(`{"text":"@_user_1"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
					Id:  &larkim.UserId{OpenId: stringPtr("ou_bot")},
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, botIdentitySet(BotIdentity{OpenID: "ou_bot"}))
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

func TestParseMessageEventIgnoresOtherMentionsWhenBotIdentityConfigured(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"@_user_1 panes"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
					Id:  &larkim.UserId{OpenId: stringPtr("ou_coworker")},
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, botIdentitySet(BotIdentity{OpenID: "ou_bot"}))
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if message.IsAppMention {
		t.Fatal("IsAppMention = true, want false")
	}
	if message.Text != "@_user_1 panes" {
		t.Fatalf("text = %q, want @_user_1 panes", message.Text)
	}
}

func TestParseMessageEventMatchesBotIdentityAcrossIDTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		identity BotIdentity
		mention  *larkim.UserId
	}{
		{
			name:     "open_id",
			identity: BotIdentity{OpenID: "ou_bot"},
			mention:  &larkim.UserId{OpenId: stringPtr("ou_bot")},
		},
		{
			name:     "user_id",
			identity: BotIdentity{UserID: "cli_user_bot"},
			mention:  &larkim.UserId{UserId: stringPtr("cli_user_bot")},
		},
		{
			name:     "union_id",
			identity: BotIdentity{UnionID: "on_bot"},
			mention:  &larkim.UserId{UnionId: stringPtr("on_bot")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			event := &larkim.P2MessageReceiveV1{
				EventV2Base: &larkevent.EventV2Base{},
				Event: &larkim.P2MessageReceiveV1Data{
					Message: &larkim.EventMessage{
						MessageId:   stringPtr("om_1"),
						ChatId:      stringPtr("oc_group_1"),
						ChatType:    stringPtr("group"),
						MessageType: stringPtr("text"),
						Content:     stringPtr(`{"text":"@_user_1 panes"}`),
						Mentions: []*larkim.MentionEvent{{
							Key: stringPtr("@_user_1"),
							Id:  tc.mention,
						}},
					},
				},
			}

			message, ok, err := parseMessageEvent(event, botIdentitySet(tc.identity))
			if err != nil {
				t.Fatalf("parseMessageEvent() error = %v", err)
			}
			if !ok {
				t.Fatal("ok = false, want true")
			}
			if !message.IsAppMention {
				t.Fatal("IsAppMention = false, want true")
			}
		})
	}
}

func TestParseMessageEventFallsBackToAnyMentionWithoutBotIdentity(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"@_user_1 panes"}`),
				Mentions: []*larkim.MentionEvent{{
					Key: stringPtr("@_user_1"),
					Id:  &larkim.UserId{OpenId: stringPtr("ou_any")},
				}},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, nil)
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !message.IsAppMention {
		t.Fatal("IsAppMention = false, want true fallback")
	}
}

func TestParseMessageEventStripsLeadingMentionPrefixWhenPrefixAddressesBot(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"@_user_2 @_user_1 panes @_user_2"}`),
				Mentions: []*larkim.MentionEvent{
					{
						Key: stringPtr("@_user_2"),
						Id:  &larkim.UserId{OpenId: stringPtr("ou_coworker")},
					},
					{
						Key: stringPtr("@_user_1"),
						Id:  &larkim.UserId{OpenId: stringPtr("ou_bot")},
					},
				},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, botIdentitySet(BotIdentity{OpenID: "ou_bot"}))
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !message.IsAppMention {
		t.Fatal("IsAppMention = false, want true")
	}
	if message.Text != "panes @_user_2" {
		t.Fatalf("text = %q, want panes @_user_2", message.Text)
	}
}

func TestParseMessageEventLeavesLeadingNonBotMentionWhenBotMentionIsNotInPrefix(t *testing.T) {
	t.Parallel()

	event := &larkim.P2MessageReceiveV1{
		EventV2Base: &larkevent.EventV2Base{},
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_1"),
				ChatId:      stringPtr("oc_group_1"),
				ChatType:    stringPtr("group"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"@_user_2 panes @_user_1"}`),
				Mentions: []*larkim.MentionEvent{
					{
						Key: stringPtr("@_user_2"),
						Id:  &larkim.UserId{OpenId: stringPtr("ou_coworker")},
					},
					{
						Key: stringPtr("@_user_1"),
						Id:  &larkim.UserId{OpenId: stringPtr("ou_bot")},
					},
				},
			},
		},
	}

	message, ok, err := parseMessageEvent(event, botIdentitySet(BotIdentity{OpenID: "ou_bot"}))
	if err != nil {
		t.Fatalf("parseMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !message.IsAppMention {
		t.Fatal("IsAppMention = false, want true")
	}
	if message.Text != "@_user_2 panes @_user_1" {
		t.Fatalf("text = %q, want @_user_2 panes @_user_1", message.Text)
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
