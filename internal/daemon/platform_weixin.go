//go:build !no_weixin

package daemon

import (
	"fmt"
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

const (
	defaultWeixinBaseURL    = "https://ilinkai.weixin.qq.com"
	defaultWeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
)

func init() {
	RegisterPlatform("weixin", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, store *Store) (platformAdapter, error) {
			return newWeixinAdapter(cfg, stderr, store)
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.WeixinToken) == "" {
				return tmuxconn.UsageError("daemon run requires --weixin-token, TMUXCONN_WEIXIN_TOKEN, or [daemon.weixin].token in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.WeixinToken) == "" {
				return tmuxconn.UsageError("weixin token is required; pass --weixin-token, TMUXCONN_WEIXIN_TOKEN, or [daemon.weixin].token in config")
			}
			if _, err := fmt.Fprintf(stdout, "weixin token: ok\nweixin api base: %s\nweixin cdn base: %s\n", cfg.WeixinBaseURL, cfg.WeixinCDNBaseURL); err != nil {
				return err
			}
			lines := []string{
				"weixin login: use `tmux-connect daemon weixin setup` to fetch or bind an iLink bearer token",
				"weixin first message: the operator must send one WeChat message first so tmux-connect can store context_token",
			}
			if cfg.WeixinRouteTag != "" {
				lines = append(lines, "weixin route tag: configured")
			}
			return writeDoctorLines(stdout, lines...)
		},
	})
}
