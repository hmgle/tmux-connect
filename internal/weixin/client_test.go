package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeStateStore struct {
	values map[string]string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(t *testing.T, req *http.Request, body any, headers map[string]string) (*http.Response, error) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	httpHeaders := make(http.Header, len(headers))
	for key, value := range headers {
		httpHeaders.Set(key, value)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     httpHeaders,
		Body:       io.NopCloser(bytes.NewReader(payload)),
		Request:    req,
	}, nil
}

func (s *fakeStateStore) GetPlatformRuntimeState(_ context.Context, platform string, scope string, entityID string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[platform+"|"+scope+"|"+entityID], nil
}

func (s *fakeStateStore) SetPlatformRuntimeState(_ context.Context, platform string, scope string, entityID string, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[platform+"|"+scope+"|"+entityID] = value
	return nil
}

func TestClientGetUpdatesAndToMessageEventPersistState(t *testing.T) {
	t.Parallel()

	store := &fakeStateStore{}
	requestSeen := false
	clientHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ilink/bot/getupdates" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		requestSeen = true
		if got := r.Header.Get("AuthorizationType"); got != "ilink_bot_token" {
			t.Fatalf("AuthorizationType = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if strings.TrimSpace(r.Header.Get("X-WECHAT-UIN")) == "" {
			t.Fatal("X-WECHAT-UIN header is empty")
		}
		return jsonResponse(t, r, getUpdatesResp{
			Ret:           0,
			GetUpdatesBuf: "cursor-1",
			Msgs: []weixinMessage{{
				MessageID:    12,
				FromUserID:   "user@im.wechat",
				MessageType:  messageTypeUser,
				CreateTimeMs: time.Now().UnixMilli(),
				ContextToken: "ctx-1",
				ItemList: []messageItem{{
					Type:     messageItemText,
					TextItem: &textItem{Text: "/panes"},
				}},
			}},
		}, nil)
	})}

	client, err := NewClient(ClientConfig{
		Token:      "test-token",
		BaseURL:    "https://example.test",
		CDNBaseURL: "https://example.test",
		HTTPClient: clientHTTP,
		Stderr:     io.Discard,
		Store:      store,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.api.getUpdates(context.Background(), "")
	if err != nil {
		t.Fatalf("getUpdates() error = %v", err)
	}
	if err := client.setRuntimeState(context.Background(), runtimeStateScopeCursor, "", resp.GetUpdatesBuf); err != nil {
		t.Fatalf("setRuntimeState(cursor) error = %v", err)
	}
	got, ok, err := client.toMessageEvent(context.Background(), &resp.Msgs[0])
	if err != nil {
		t.Fatalf("toMessageEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("toMessageEvent() ok = false, want true")
	}
	if !requestSeen {
		t.Fatal("getupdates request was not seen")
	}
	if got.ChatID != "user@im.wechat" || got.MessageID != "12" || got.Text != "/panes" {
		t.Fatalf("event = %#v", got)
	}
	if store.values["weixin|cursor|"] != "cursor-1" {
		t.Fatalf("cursor = %q, want cursor-1", store.values["weixin|cursor|"])
	}
	if store.values["weixin|context_token|user@im.wechat"] != "ctx-1" {
		t.Fatalf("context token = %q, want ctx-1", store.values["weixin|context_token|user@im.wechat"])
	}
}

func TestClientGetUpdatesRejectsBusinessError(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, r, getUpdatesResp{
			Ret:     0,
			Errcode: 4001,
			Errmsg:  "invalid token",
		}, nil)
	})}

	client, err := NewClient(ClientConfig{
		Token:      "test-token",
		BaseURL:    "https://example.test",
		CDNBaseURL: "https://example.test",
		HTTPClient: clientHTTP,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.api.getUpdates(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("getUpdates() error = %v, want business error", err)
	}
}

func TestClientGetUpdatesAllowsSessionExpiredResponse(t *testing.T) {
	t.Parallel()

	clientHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(t, r, getUpdatesResp{
			Ret:     1,
			Errcode: sessionExpiredErrcode,
			Errmsg:  "session expired",
		}, nil)
	})}

	client, err := NewClient(ClientConfig{
		Token:      "test-token",
		BaseURL:    "https://example.test",
		CDNBaseURL: "https://example.test",
		HTTPClient: clientHTTP,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	resp, err := client.api.getUpdates(context.Background(), "")
	if err != nil {
		t.Fatalf("getUpdates() error = %v", err)
	}
	if resp.Errcode != sessionExpiredErrcode {
		t.Fatalf("errcode = %d, want %d", resp.Errcode, sessionExpiredErrcode)
	}
}

func TestClientSendTextUsesStoredContextToken(t *testing.T) {
	t.Parallel()

	store := &fakeStateStore{values: map[string]string{
		"weixin|context_token|user@im.wechat": "ctx-1",
	}}
	clientHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req sendMessageReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.Msg.ToUserID != "user@im.wechat" || req.Msg.ContextToken != "ctx-1" {
			t.Fatalf("request = %#v", req.Msg)
		}
		if len(req.Msg.ItemList) != 1 || req.Msg.ItemList[0].TextItem == nil || req.Msg.ItemList[0].TextItem.Text != "hello" {
			t.Fatalf("item list = %#v", req.Msg.ItemList)
		}
		return jsonResponse(t, r, sendMessageResp{Ret: 0}, nil)
	})}

	client, err := NewClient(ClientConfig{
		Token:      "test-token",
		BaseURL:    "https://example.test",
		CDNBaseURL: "https://example.test",
		HTTPClient: clientHTTP,
		Store:      store,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.SendText(context.Background(), "user@im.wechat", "hello"); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
}

func TestClientSendImageUploadsMediaAndCaption(t *testing.T) {
	t.Parallel()

	store := &fakeStateStore{values: map[string]string{
		"weixin|context_token|user@im.wechat": "ctx-1",
	}}
	uploaded := false
	sendMessageCalls := 0
	clientHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/ilink/bot/getuploadurl":
			return jsonResponse(t, r, getUploadURLResponse{UploadParam: "upload-param"}, nil)
		case "/upload":
			uploaded = true
			return jsonResponse(t, r, map[string]any{}, map[string]string{"x-encrypted-param": "download-param"})
		case "/ilink/bot/sendmessage":
			sendMessageCalls++
			var req sendMessageReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			switch sendMessageCalls {
			case 1:
				if len(req.Msg.ItemList) != 1 {
					t.Fatalf("first send item count = %d, want 1", len(req.Msg.ItemList))
				}
				if req.Msg.ItemList[0].ImageItem == nil || req.Msg.ItemList[0].ImageItem.Media == nil || req.Msg.ItemList[0].ImageItem.Media.EncryptQueryParam != "download-param" {
					t.Fatalf("first send image item = %#v", req.Msg.ItemList[0].ImageItem)
				}
				if req.Msg.ItemList[0].TextItem != nil {
					t.Fatalf("first send text item = %#v, want nil", req.Msg.ItemList[0].TextItem)
				}
			case 2:
				if len(req.Msg.ItemList) != 1 {
					t.Fatalf("second send item count = %d, want 1", len(req.Msg.ItemList))
				}
				if req.Msg.ItemList[0].TextItem == nil || req.Msg.ItemList[0].TextItem.Text != "pane snapshot" {
					t.Fatalf("second send text item = %#v", req.Msg.ItemList[0].TextItem)
				}
				if req.Msg.ItemList[0].ImageItem != nil {
					t.Fatalf("second send image item = %#v, want nil", req.Msg.ItemList[0].ImageItem)
				}
			default:
				t.Fatalf("unexpected sendmessage call #%d", sendMessageCalls)
			}
			return jsonResponse(t, r, sendMessageResp{Ret: 0}, nil)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return nil, nil
	})}

	client, err := NewClient(ClientConfig{
		Token:      "test-token",
		BaseURL:    (&url.URL{Scheme: "https", Host: "api.example.test"}).String(),
		CDNBaseURL: (&url.URL{Scheme: "https", Host: "api.example.test"}).String(),
		HTTPClient: clientHTTP,
		Store:      store,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.SendImage(context.Background(), "user@im.wechat", "pane-snapshot.png", []byte("fake-image-bytes"), "pane snapshot"); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}
	if !uploaded {
		t.Fatal("expected CDN upload request")
	}
	if sendMessageCalls != 2 {
		t.Fatalf("sendmessage calls = %d, want 2", sendMessageCalls)
	}
}
