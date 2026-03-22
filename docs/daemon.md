# Daemon Configuration Reference

The relay daemon connects tmux panes to chat platforms. It shares the same
config file and precedence rules as the CLI: flags > env vars > TOML.

## Config File

```toml
[tmux]
socket = "work"

[daemon]
platform = "telegram"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["123456789"]
poll_timeout = "20s"
snapshot_lines = 120
plain_text_mode = "type"
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
```

## Common Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--platform` | required | `telegram`, `slack`, `discord`, or `whatsapp` |
| `--db PATH` | required | SQLite database path |
| `--allow-chat ID` | — | Restrict access to specific chat IDs (repeatable) |
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

### Telegram-specific

| Flag | Default | Description |
|------|---------|-------------|
| `--telegram-token` | — | Bot API token (or `TMUXCONN_TELEGRAM_TOKEN`) |
| `--telegram-snapshot-theme` | `dark` | `dark` or `light` |
| `--telegram-snapshot-font-size` | `14` | Font size for snapshot images |
| `--telegram-snapshot-font-file` | built-in gomono | Custom font path |
| `--telegram-api-base` | — | Custom Telegram API base URL |

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

# Show SQLite record counts and managed pane count
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
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
current pane selection, sessions, and message links. Schema is versioned via
`PRAGMA user_version`.

## Platform Guides

For platform-specific setup, commands, and operational notes:

- [Telegram](./telegram.md)
- [Slack](./slack.md)
- [Discord](./discord.md)
- [WhatsApp](./whatsapp.md)
