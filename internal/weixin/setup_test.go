package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFetchBotQRCode(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ilink/bot/get_bot_qrcode" {
			t.Fatalf("path = %s", req.URL.Path)
		}
		if got := req.URL.Query().Get("bot_type"); got != "3" {
			t.Fatalf("bot_type = %q", got)
		}
		payload, _ := json.Marshal(BotQRCode{QRCode: "qr-key", QRCodeImgContent: "https://example.test/qr"})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	got, err := FetchBotQRCode(context.Background(), client, "https://ilinkai.weixin.qq.com", "", "")
	if err != nil {
		t.Fatalf("FetchBotQRCode() error = %v", err)
	}
	if got.QRCode != "qr-key" || got.QRCodeImgContent != "https://example.test/qr" {
		t.Fatalf("qr = %#v", got)
	}
}

func TestPollQRCodeStatusTimeoutMapsToWait(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})}
	got, err := PollQRCodeStatus(context.Background(), client, "https://ilinkai.weixin.qq.com", "qr-key", "")
	if err != nil {
		t.Fatalf("PollQRCodeStatus() error = %v", err)
	}
	if got.Status != "wait" {
		t.Fatalf("status = %#v", got)
	}
}

func TestVerifyTokenSetsHeaders(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("AuthorizationType"); got != "ilink_bot_token" {
			t.Fatalf("AuthorizationType = %q", got)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q", got)
		}
		if strings.TrimSpace(req.Header.Get("X-WECHAT-UIN")) == "" {
			t.Fatal("X-WECHAT-UIN is empty")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ret":0}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	if err := VerifyToken(context.Background(), client, "https://ilinkai.weixin.qq.com", "token-1", ""); err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
}
