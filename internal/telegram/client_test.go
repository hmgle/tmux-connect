package telegram

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
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
		if _, ok := payload["parse_mode"]; ok {
			t.Fatalf("unexpected parse_mode in %#v", payload)
		}
		if _, ok := payload["reply_parameters"]; ok {
			t.Fatalf("unexpected reply_parameters in %#v", payload)
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
		replyParameters, ok := payload["reply_parameters"].(map[string]any)
		if !ok {
			t.Fatalf("reply_parameters missing in %#v", payload)
		}
		if replyParameters["message_id"] != float64(42) {
			t.Fatalf("reply_parameters.message_id = %#v, want 42", replyParameters["message_id"])
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

func TestSendMessageWithParseMode(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["parse_mode"] != "HTML" {
			t.Fatalf("parse_mode = %#v, want %q", payload["parse_mode"], "HTML")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 101,
				"text":       "<b>hello</b>",
				"chat": map[string]any{
					"id":   1,
					"type": "private",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	message, err := client.SendMessage(context.Background(), 1, "<b>hello</b>", SendOptions{ParseMode: ParseModeHTML})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.MessageID != 101 {
		t.Fatalf("message_id = %d, want 101", message.MessageID)
	}
}

func TestSendPhoto(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType() error = %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("mediaType = %q, want multipart/form-data", mediaType)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])

		fields := map[string]string{}
		var fileName string
		var fileBody []byte
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() error = %v", err)
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if part.FormName() == "photo" {
				fileName = part.FileName()
				fileBody = data
				continue
			}
			fields[part.FormName()] = string(data)
		}

		if fields["chat_id"] != "1" {
			t.Fatalf("chat_id = %q, want 1", fields["chat_id"])
		}
		if fields["caption"] != "pane snapshot" {
			t.Fatalf("caption = %q, want pane snapshot", fields["caption"])
		}
		if fields["reply_parameters"] != "{\"message_id\":42}" {
			t.Fatalf("reply_parameters = %q, want %q", fields["reply_parameters"], "{\"message_id\":42}")
		}
		if fileName != "snapshot.png" {
			t.Fatalf("filename = %q, want snapshot.png", fileName)
		}
		if string(fileBody) != "pngbytes" {
			t.Fatalf("file body = %q, want pngbytes", string(fileBody))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 102,
				"chat": map[string]any{
					"id":   1,
					"type": "private",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient("token", WithBaseURL(server.URL))
	message, err := client.SendPhoto(context.Background(), 1, "snapshot.png", []byte("pngbytes"), "pane snapshot", SendOptions{ReplyToMessageID: 42})
	if err != nil {
		t.Fatalf("SendPhoto() error = %v", err)
	}
	if message.MessageID != 102 {
		t.Fatalf("message_id = %d, want 102", message.MessageID)
	}
}
