# Telegram Setup and Operations

## Prerequisites

- `tmux` is installed and the target pane already exists
- `sqlite3` is installed and available in `PATH`
- you have a Telegram bot token from BotFather
- you know the Telegram `chat_id` you want to allow if you plan to use `--allow-chat`

## Optional Local Preparation

You can pre-manage a pane locally, or let Telegram manage it on first `/select`.

```bash
./tmux-connect attach --pane %5 --agent codex --label backend
./tmux-connect inspect --pane %5
```

## Start the Daemon

`tmux-connect` also reads `$XDG_CONFIG_HOME/tmux-connect/config.toml` by default
(falling back to `$HOME/.config/tmux-connect/config.toml`). Flags override
environment variables, and environment variables override the TOML file.

Using TOML config:

```toml
[daemon]
platform = "telegram"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["123456789"]
snapshot_lines = 120
plain_text_mode = "type"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"

[daemon.telegram]
token = "123456:example-token"
snapshot_theme = "light"
snapshot_font_size = 16
snapshot_font_file = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
```

```bash
./tmux-connect daemon run
```

Using environment variables:

```bash
export TMUXCONN_TELEGRAM_TOKEN=123456:example-token
export TMUXCONN_DB_PATH="$HOME/.tmux-connect/tmux-connect.db"
export TMUXCONN_TELEGRAM_SNAPSHOT_THEME=light
export TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE=16

./tmux-connect daemon run --allow-chat 123456789
```

Using explicit flags:

```bash
./tmux-connect daemon run \
  --telegram-token 123456:example-token \
  --db ~/.tmux-connect/tmux-connect.db \
  --plain-text-mode execute \
  --plain-text-echo snapshot \
  --telegram-snapshot-theme light \
  --telegram-snapshot-font-size 16 \
  --telegram-snapshot-font-file /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf \
  --follow-lines 80 \
  --follow-min-interval 700ms \
  --allow-chat 123456789
```

## Daemon Note

Run one active `tmux-connect daemon run` per bot token.

If you want to control multiple machines, the simple setup is one Telegram bot
per machine.

Useful daemon flags:

- `--telegram-token TOKEN`
- `--db PATH`
- `--allow-chat CHAT_ID`
- `--poll-timeout 20s`
- `--snapshot-lines 120`
- `--plain-text-mode type|execute`
- `--plain-text-echo off|snapshot`
- `--plain-text-echo-lines 12`
- `--plain-text-echo-delay 250ms`
- `--plain-text-echo-timeout 2s`
- `--telegram-snapshot-theme dark|light`
- `--telegram-snapshot-font-size 14`
- `--telegram-snapshot-font-file /path/to/font.ttf`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`
- `--telegram-api-base URL`

Supported environment variables:

- `TMUXCONN_TELEGRAM_TOKEN`
- `TMUXCONN_DB_PATH`
- `TMUXCONN_PLAIN_TEXT_MODE`
- `TMUXCONN_PLAIN_TEXT_ECHO`
- `TMUXCONN_PLAIN_TEXT_ECHO_LINES`
- `TMUXCONN_PLAIN_TEXT_ECHO_DELAY`
- `TMUXCONN_PLAIN_TEXT_ECHO_TIMEOUT`
- `TMUXCONN_TELEGRAM_SNAPSHOT_THEME`
- `TMUXCONN_TELEGRAM_SNAPSHOT_FONT_SIZE`
- `TMUXCONN_TELEGRAM_SNAPSHOT_FONT_FILE`
- `TMUXCONN_TELEGRAM_API_BASE`
- `TMUXCONN_FOLLOW_DEBUG`

## Health Checks

Validate runtime prerequisites:

```bash
./tmux-connect daemon doctor --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
```

Inspect stored state and current managed pane count:

```bash
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

## Telegram Commands

- `/start`
- `/help`
- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>` for explicit text sends, especially if the text itself starts with `/`
- `/keys <key...>` or `/key <key...>` for tmux key names such as `C-c`, `PageUp`, `F1`, or `M-x`
- `/enter [text]`
- `/ctrlc` or `/ctrl-c`
- `/follow on [interval]|off`

Plain text without a slash always targets the current pane. By default it stays
in raw `type` mode, so it does not press Enter automatically. Set
`plain_text_mode = "execute"` or pass `--plain-text-mode execute` to make bare
text send and press Enter in one step. When snapshot echo is enabled, the
daemon replies with a short text snapshot after visible pane output changes.

If `/select`, `/unmanage`, `/send`, `/keys`, or `/follow` is sent without arguments, the bot prompts with Telegram `ForceReply` and waits for the missing value.

## Typical Flow

1. Start the daemon.
2. Open the bot in Telegram and send `/panes`.
3. Select a pane with `/select %5`.
4. Check the current selection with `/current`.
5. Read output with `/snapshot` or `/snapshot text`.
6. Send input by typing `continue`, or let bare text execute immediately if execute mode is enabled.
7. Press Enter with `/enter`, or run a one-shot command with `/enter continue`.
8. Send control keys with `/keys C-c` when needed.
9. Enable follow mode with `/follow on` or `/follow on 2s`.

## Operational Notes

- if `--allow-chat` is set, chats outside the allowlist are rejected
- `/select` automatically attaches the pane if it is not already managed
- plain text always targets the current pane; in the default `type` mode it stays raw, while `plain_text_mode = "execute"` makes bare text send and press Enter
- `/enter <text>` sends text and presses Enter in one step
- `/keys` sends tmux key names such as `Enter`, `C-c`, `Escape`, arrows, `PageUp`, `F1`-`F12`, `C-a`-`C-z`, or `M-x`
- `/send` remains available when you need to send text that starts with `/` or when you want raw text even while execute mode is enabled
- `/clear` clears only the current pane for the current chat and disables that chat's follow session
- `/snapshot` defaults to `image`; `text` skips image rendering and sends plain text
- snapshot images default to the built-in `gomono` font, `14` pt, and the `dark` theme
- `/unmanage` clears chat bindings and stops follow sessions that point to that pane
- if the current pane disappears or becomes unmanaged, the daemon clears that chat's current-pane state
- follow mode is one active subscription per chat
