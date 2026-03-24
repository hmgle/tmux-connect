//go:build !no_weixin

package daemon

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hmgle/tmux-connect/internal/config"
)

type daemonRoundTripFunc func(*http.Request) (*http.Response, error)

func (f daemonRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunWeixinBindWritesConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	loaded := config.Loaded{
		Path:   configPath,
		Config: config.File{Daemon: config.Daemon{DB: stringPtr(filepath.Join(t.TempDir(), "tmuxconn.db"))}},
	}
	deps := weixinCLIDeps{
		httpClient: func() *http.Client {
			return &http.Client{Transport: daemonRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ret":0}`)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			})}
		},
		sleep: func(time.Duration) {},
	}

	var stdout bytes.Buffer
	err := runWeixinCLIWithDeps(context.Background(), &stdout, &bytes.Buffer{}, loaded, []string{"bind", "--token", "token-1"}, deps)
	if err != nil {
		t.Fatalf("runWeixinCLIWithDeps() error = %v", err)
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if saved.Config.Daemon.Platform == nil || *saved.Config.Daemon.Platform != "weixin" {
		t.Fatalf("platform = %#v", saved.Config.Daemon.Platform)
	}
	if saved.Config.Daemon.Weixin.Token == nil || *saved.Config.Daemon.Weixin.Token != "token-1" {
		t.Fatalf("token = %#v", saved.Config.Daemon.Weixin.Token)
	}
	if !strings.Contains(stdout.String(), "configuration updated") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWeixinQRSetupSetsAllowChatWhenEmpty(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	loaded := config.Loaded{Path: configPath}
	call := 0
	deps := weixinCLIDeps{
		httpClient: func() *http.Client {
			return &http.Client{Transport: daemonRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				call++
				switch req.URL.Path {
				case "/ilink/bot/get_bot_qrcode":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"qrcode":"qr-key","qrcode_img_content":"https://example.test/qr"}`)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				case "/ilink/bot/get_qrcode_status":
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"status":"confirmed","bot_token":"token-qr","ilink_user_id":"user@im.wechat","baseurl":"https://api.weixin.example"}`)),
						Header:     make(http.Header),
						Request:    req,
					}, nil
				default:
					t.Fatalf("unexpected path %s", req.URL.Path)
					return nil, nil
				}
			})}
		},
		sleep: func(time.Duration) {},
	}

	err := runWeixinCLIWithDeps(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, loaded, []string{"setup", "--timeout", "2s"}, deps)
	if err != nil {
		t.Fatalf("runWeixinCLIWithDeps() error = %v", err)
	}
	if call < 2 {
		t.Fatalf("http call count = %d, want >= 2", call)
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if saved.Config.Daemon.AllowChats == nil || len(*saved.Config.Daemon.AllowChats) != 1 || (*saved.Config.Daemon.AllowChats)[0] != "weixin:user@im.wechat" {
		t.Fatalf("allow chats = %#v", saved.Config.Daemon.AllowChats)
	}
	if saved.Config.Daemon.Weixin.Token == nil || *saved.Config.Daemon.Weixin.Token != "token-qr" {
		t.Fatalf("weixin token = %#v", saved.Config.Daemon.Weixin.Token)
	}
}
