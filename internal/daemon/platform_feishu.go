//go:build !no_feishu

package daemon

import (
	"io"
	"strings"

	"github.com/hmgle/tmux-connect/internal/tmuxconn"
)

func init() {
	RegisterPlatform("feishu", platformRegistration{
		factory: func(cfg Config, stderr io.Writer, _ *Store) (platformAdapter, error) {
			return newFeishuAdapter(cfg, stderr)
		},
		validateRun: func(cfg Config) error {
			if strings.TrimSpace(cfg.FeishuAppID) == "" || strings.TrimSpace(cfg.FeishuAppSecret) == "" {
				return tmuxconn.UsageError("daemon run requires --feishu-app-id/--feishu-app-secret, TMUXCONN_FEISHU_APP_ID/TMUXCONN_FEISHU_APP_SECRET, or [daemon.feishu].app_id/[daemon.feishu].app_secret in config")
			}
			return nil
		},
		doctor: func(stdout io.Writer, cfg Config) error {
			if strings.TrimSpace(cfg.FeishuAppID) == "" {
				return tmuxconn.UsageError("feishu app id is required; pass --feishu-app-id, TMUXCONN_FEISHU_APP_ID, or [daemon.feishu].app_id in config")
			}
			if strings.TrimSpace(cfg.FeishuAppSecret) == "" {
				return tmuxconn.UsageError("feishu app secret is required; pass --feishu-app-secret, TMUXCONN_FEISHU_APP_SECRET, or [daemon.feishu].app_secret in config")
			}
			if err := writeDoctorLines(
				stdout,
				"feishu app credentials: ok",
				"feishu bot capability: enable bot ability and subscribe to im.message.receive_v1",
				"feishu group behavior: the bot only handles @mentions in group chats",
			); err != nil {
				return err
			}
			if strings.TrimSpace(cfg.FeishuBotOpenID) != "" || strings.TrimSpace(cfg.FeishuBotUserID) != "" || strings.TrimSpace(cfg.FeishuBotUnionID) != "" {
				return writeDoctorLines(stdout, "feishu bot identity: ok (precise group @mention matching enabled)")
			}
			return writeDoctorLines(stdout, "feishu bot identity: optional; set --feishu-bot-open-id/--feishu-bot-user-id/--feishu-bot-union-id to avoid mistaking @other-user as a bot command")
		},
	})
}
