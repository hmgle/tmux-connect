# Daemon Configuration Reference

The relay daemon connects tmux panes to chat platforms. It shares the same
config file and precedence rules as the CLI: flags > env vars > TOML.

The set of usable platforms depends on how the binary was built. Run
`./tmux-connect daemon help` to see which platforms are compiled into the
current binary.

## Runtime Model

A single `tmux-connect daemon run` process handles exactly one remote platform.
The binary may be compiled with multiple platforms, but `--platform` still
selects one platform for that process.

If you want Telegram + Slack + Discord at the same time, start multiple daemon
instances:

```bash
./tmux-connect daemon run --platform telegram ...
./tmux-connect daemon run --platform slack ...
./tmux-connect daemon run --platform discord ...
```

This is the supported way to run multiple platforms today.

## Config File

```toml
[tmux]
socket = "work"

[daemon]
platform = "telegram"
db = "/home/user/.tmux-connect/tmux-connect.db"
# optional allowlist; remove this line or leave it empty to allow any Telegram chat
allow_chats = ["telegram:123456789"]
poll_timeout = "20s"
snapshot_lines = 120
plain_text_mode = "execute"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"
follow_debug = false

[daemon.telegram]
token = "123456:example-token"
snapshot_theme = "dark"
snapshot_font_size = 14
snapshot_font_file = "/path/to/font.ttf"

[daemon.feishu]
app_id = "cli_xxx"
app_secret = "secret_xxx"

[daemon.slack]
bot_token = "xoxb-..."
app_token = "xapp-..."
command_prefix = "tmux:"

[daemon.discord]
token = "discord-bot-token"
command_prefix = "tmux:"

[daemon.whatsapp]
session_db = "/home/user/.tmux-connect/whatsapp-device.db"
device_name = "tmux-connect"
auto_mark_read = true

[daemon.weixin]
token = "ilink-bearer-token"
base_url = "https://ilinkai.weixin.qq.com"
cdn_base_url = "https://novac2c.cdn.weixin.qq.com/c2c"
route_tag = ""
```

This example opts into `plain_text_mode = "execute"` so bare text sends and
presses Enter immediately. The built-in default is still `type`, so keep
`type` if you want bare text without Enter.

## Common Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--platform` | build-dependent | One of the platforms compiled into the current binary |
| `--db PATH` | required | SQLite database path |
| `--allow-chat ID` | — | Optional allowlist entry. Repeatable; if omitted, any chat that can reach the bot on the selected platform may use it |
| `--poll-timeout` | `20s` | Long-poll timeout |
| `--snapshot-lines` | `120` | Default snapshot line count |
| `--plain-text-mode` | `type` | `type` (raw input) or `execute` (input + Enter) |
| `--plain-text-echo` | `snapshot` | `off` or `snapshot` (reply with pane output after execute) |
| `--plain-text-echo-lines` | `12` | Lines in echo snapshot |
| `--plain-text-echo-delay` | `250ms` | Wait before capturing echo snapshot |
| `--plain-text-echo-timeout` | `2s` | Max wait for visible output change |
| `--follow-lines` | `80` | Max lines per follow update |
| `--follow-min-interval` | `700ms` | Minimum gap between follow updates (WhatsApp defaults to `2s` if not set) |
| `--follow-debug` | `false` | Enable follow debug logging |

`--platform` is singular on purpose: each daemon process runs one platform.
When a binary includes multiple platforms, `--platform` chooses which adapter
to start for this process.

## Allowlist Format

`--allow-chat` and `[daemon].allow_chats` are optional. Use them only when you
want to restrict the daemon to specific chats.

- if the allowlist is empty, any chat that can reach the bot on the selected
  platform is accepted
- platform-scoped values such as `telegram:123456789`, `discord:123...`,
  `feishu:oc_xxx`, `slack:C123`, `whatsapp:8613...@s.whatsapp.net`, or
  `weixin:user@im.wechat` are recommended to avoid collisions across platforms
- raw IDs without the `platform:` prefix still work when they are unique, but
  the prefixed form is clearer in shared configs

If you do not know the exact value yet, temporarily leave the allowlist empty,
send one test message to the bot, then inspect the latest `message_log.chat_id`
for that platform in the daemon SQLite database. Each platform guide below also
documents the usual ID format and, where practical, easier ways to find it.

### Telegram-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--telegram-token` | — | Bot API token (or `TMUXCONN_TELEGRAM_TOKEN`) |
| `--telegram-snapshot-theme` | `dark` | `dark` or `light` |
| `--telegram-snapshot-font-size` | `14` | Font size for snapshot images |
| `--telegram-snapshot-font-file` | built-in gomono | Custom font path |
| `--telegram-api-base` | — | Custom Telegram API base URL |

### Feishu-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--feishu-app-id` | — | Feishu app ID (or `TMUXCONN_FEISHU_APP_ID`) |
| `--feishu-app-secret` | — | Feishu app secret (or `TMUXCONN_FEISHU_APP_SECRET`) |

### Slack-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--slack-bot-token` | — | Bot token (`xoxb-...`) |
| `--slack-app-token` | — | App-level token (`xapp-...`) for Socket Mode |
| `--slack-command-prefix` | `tmux:` | Command prefix in messages |

### Discord-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--discord-token` | — | Bot token |
| `--discord-command-prefix` | `tmux:` | Prefix for text-based commands |

### WhatsApp-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--whatsapp-session-db` | — | Local session database path (or `TMUXCONN_WHATSAPP_SESSION_DB`) |
| `--whatsapp-device-name` | `tmux-connect` | Paired device display name |
| `--whatsapp-auto-mark-read` | `true` | Auto-mark messages as read |
| `--whatsapp-allow-self-chat` | `false` | Enable experimental self-chat mode |

### Weixin-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--weixin-token` | — | iLink bot bearer token (or `TMUXCONN_WEIXIN_TOKEN`) |
| `--weixin-base-url` | `https://ilinkai.weixin.qq.com` | iLink API base URL |
| `--weixin-cdn-base-url` | `https://novac2c.cdn.weixin.qq.com/c2c` | iLink CDN base URL |
| `--weixin-route-tag` | — | Optional `SKRouteTag` request header |

## Plain Text Mode

By default, bare text is sent to the current pane in `type` mode — input only,
no Enter. Set `--plain-text-mode execute` to treat bare text as send + Enter.

When execute mode is combined with `--plain-text-echo snapshot`, the daemon
replies with a short text snapshot after visible pane output changes.

Use `/send <text>` when you want raw input even with execute mode enabled.

## Diagnostic Commands

```bash
# Validate token, SQLite store, and tmux access
./tmux-connect daemon doctor --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"

# Run with Feishu websocket event delivery
./tmux-connect daemon run --platform feishu --feishu-app-id "$TMUXCONN_FEISHU_APP_ID" --feishu-app-secret "$TMUXCONN_FEISHU_APP_SECRET"

# Run two platforms at the same time by starting two daemon processes
./tmux-connect daemon run --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" --db ~/.tmux-connect/tmux-connect.db
./tmux-connect daemon run --platform slack --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN" --db ~/.tmux-connect/tmux-connect.db

# Show SQLite record counts and managed pane count
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db

# Run with Weixin iLink
./tmux-connect daemon run --platform weixin --weixin-token "$TMUXCONN_WEIXIN_TOKEN" --db ~/.tmux-connect/tmux-connect.db --allow-chat weixin:user@im.wechat

# Configure Weixin iLink through QR login
./tmux-connect daemon weixin setup

# Or bind an existing token
./tmux-connect daemon weixin bind --token "$TMUXCONN_WEIXIN_TOKEN"
```

## Recovery Model

tmux remains the source of truth for pane identity and management metadata.
The bridge writes recovery state onto the pane with tmux user options:

- `@tmuxconn_managed=1`
- `@tmuxconn_mode=relay`
- `@tmuxconn_agent=<agent>`
- `@tmuxconn_label=<label>`
- `@tmuxconn_created_by=manual-attach`
- `@tmuxconn_last_activity_unix=<unix timestamp>`

The remote daemon stores platform chat state in SQLite, including bindings,
current pane selection, sessions, message links, and platform runtime state
such as the Weixin iLink cursor and per-user `context_token`. Schema is
versioned via `PRAGMA user_version`.

## Platform Guides

For platform-specific setup, commands, and operational notes:

- [Telegram](./telegram.md)
- [Feishu](./feishu.md)
- [Slack](./slack.md)
- [Discord](./discord.md)
- [WhatsApp](./whatsapp.md)
- [Weixin](./weixin.md)
