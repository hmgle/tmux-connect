package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetUpdates(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotTimeout int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		gotTimeout = int(payload["timeout"].(float64))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []map[string]any{
				{
					"update_id": 42,
					"message": map[string]any{
						"message_id": 7,
						"text":       "/panes",
						"chat": map[string]any{
							"id":   123,
							"type": "private",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	updates, err := client.GetUpdates(context.Background(), 10, 5*time.Second)
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}
	if gotPath != "/bottoken/getUpdates" {
		t.Fatalf("path = %q, want %q", gotPath, "/bottoken/getUpdates")
	}
	if gotTimeout != 5 {
		t.Fatalf("timeout = %d, want 5", gotTimeout)
	}
	if len(updates) != 1 || updates[0].Message == nil || updates[0].Message.Text != "/panes" {
		t.Fatalf("unexpected updates %#v", updates)
	}
}

func TestDrainPendingUpdates(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		result := []map[string]any{}
		if calls == 1 {
			result = []map[string]any{{"update_id": 100}, {"update_id": 101}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": result,
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	offset, err := client.DrainPendingUpdates(context.Background())
	if err != nil {
		t.Fatalf("DrainPendingUpdates() error = %v", err)
	}
	if offset != 102 {
		t.Fatalf("offset = %d, want 102", offset)
	}
}

func TestSendMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["text"] != "hello" {
			t.Fatalf("text = %#v, want %q", payload["text"], "hello")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 99,
				"text":       "hello",
				"chat": map[string]any{
					"id":   1,
					"type": "private",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	message, err := client.SendMessage(context.Background(), 1, "hello", SendOptions{})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.MessageID != 99 {
		t.Fatalf("message_id = %d, want 99", message.MessageID)
	}
}

func TestSendMessageWithReplyTarget(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["reply_to_message_id"] != float64(42) {
			t.Fatalf("reply_to_message_id = %#v, want 42", payload["reply_to_message_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 100,
				"text":       "hello",
				"chat": map[string]any{
					"id":   1,
					"type": "private",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	message, err := client.SendMessage(context.Background(), 1, "hello", SendOptions{ReplyToMessageID: 42})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.MessageID != 100 {
		t.Fatalf("message_id = %d, want 100", message.MessageID)
	}
}
