# Slack Setup and Operations

## Prerequisites

- `tmux` is installed and the target pane already exists
- no external `sqlite3` CLI is required; the daemon uses embedded SQLite
- you have a Slack app with:
  - a bot token (`xoxb-...`)
  - an app-level token (`xapp-...`) with Socket Mode enabled

## Required Slack App Settings

- enable Socket Mode
- add bot scopes:
  - `app_mentions:read`
  - `chat:write`
  - `files:write`
  - `im:history`
  - `im:read`
- subscribe to bot events:
  - `app_mention`
  - `message.im`

Socket Mode means you do not need a public webhook URL.
Slack snapshot images use Slack's current Web API file upload flow, so `files:write` is required. If you add scopes later, reinstall the app before retrying `tmux: snapshot`.

## Start the Daemon

`tmux-connect` also reads `$XDG_CONFIG_HOME/tmux-connect/config.toml` by default
(falling back to `$HOME/.config/tmux-connect/config.toml`). Flags override
environment variables, and environment variables override the TOML file.

Using TOML config:

```toml
[daemon]
platform = "slack"
db = "/home/user/.tmux-connect/tmux-connect.db"
# optional allowlist; remove this line to allow any reachable Slack DM or channel thread
allow_chats = ["slack:D12345678"]
snapshot_lines = 120
plain_text_mode = "execute"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"

[daemon.slack]
bot_token = "xoxb-..."
app_token = "xapp-..."
command_prefix = "tmux:"
```

This example intentionally uses `plain_text_mode = "execute"` so bare text in
DMs sends and presses Enter immediately. Keep `type` if you want bare text to
stay as raw input without Enter.

```bash
./tmux-connect daemon run
```

```bash
./tmux-connect daemon run \
  --platform slack \
  --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" \
  --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN" \
  --plain-text-mode execute \
  --plain-text-echo snapshot \
  --db ~/.tmux-connect/tmux-connect.db
```

Useful flags:

- `--platform slack`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
- `--slack-command-prefix PREFIX` (default: `tmux:`)
- `--db PATH`
- `--allow-chat slack:CHANNEL_OR_DM_ID`
- `--snapshot-lines 120`
- `--plain-text-mode type|execute`
- `--plain-text-echo off|snapshot`
- `--plain-text-echo-lines 12`
- `--plain-text-echo-delay 250ms`
- `--plain-text-echo-timeout 2s`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`

Supported environment variables:

- `TMUXCONN_PLATFORM=slack`
- `TMUXCONN_SLACK_BOT_TOKEN`
- `TMUXCONN_SLACK_APP_TOKEN`
- `TMUXCONN_SLACK_COMMAND_PREFIX`
- `TMUXCONN_DB_PATH`
- `TMUXCONN_PLAIN_TEXT_MODE`
- `TMUXCONN_PLAIN_TEXT_ECHO`
- `TMUXCONN_PLAIN_TEXT_ECHO_LINES`
- `TMUXCONN_PLAIN_TEXT_ECHO_DELAY`
- `TMUXCONN_PLAIN_TEXT_ECHO_TIMEOUT`
- `TMUXCONN_FOLLOW_DEBUG`

## Choosing `allow_chats` Values

`--allow-chat` and `[daemon].allow_chats` are optional. If you do not set them,
any Slack DM or channel thread that can reach the bot may use it.

- recommended format: `slack:<conversation_id>`
- DM IDs usually look like `D...`; channel IDs usually look like `C...`
- if you are unsure which conversation ID Slack is delivering, start once
  without an allowlist, send a test message, then inspect the latest
  `platform = slack` row in `message_log` and copy the exact `chat_id`

## Health Checks

```bash
./tmux-connect daemon doctor \
  --platform slack \
  --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" \
  --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN"

./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

`daemon doctor` echoes the Slack scopes the bot needs for snapshot image uploads. If `tmux: snapshot` falls back to text, check the daemon log for `reply bus send snapshot image`.

## Using the Bot

- in a DM with the bot, send commands directly
- in a channel, mention the bot once to start a managed thread
- follow-up commands can continue inside that thread without re-mentioning the bot
- in channels, use an app mention as the command prefix, for example `@tmux-connect panes`
- in DMs and managed threads, plain text always targets the current pane; default `type` mode keeps it raw, while execute mode sends text and Enter in one step
- in DMs and managed threads, prefix explicit commands with `tmux:`, for example `tmux: select %5`, `tmux: keys C-c`, or `tmux: send /start`
- slash-prefixed forms like `/panes` are still accepted by the router, but Slack may intercept them as real Slash Commands before they reach the bot

This bot intentionally does not depend on Slack Slash Commands for the main workflow. Slash invocations are awkward for thread-based control, while mention-based entry, plain-text input, and `tmux: ...` commands fit thread-based control better.

Supported commands:

- `tmux: start`
- `tmux: help`
- `tmux: panes`
- `tmux: select <pane>`
- `tmux: clear`
- `tmux: unmanage <pane>`
- `tmux: current`
- `tmux: snapshot [lines] [image|text]`
- `tmux: send <text>` for explicit text sends, especially if the text itself starts with `/`
- `tmux: keys <key...>` or `tmux: key <key...>` for tmux key names such as `C-c`, `PageUp`, `F1`, or `M-x`
- `tmux: enter [text]`
- `tmux: ctrlc` or `tmux: ctrl-c`
- `tmux: follow on [interval]|off`

If `tmux: select`, `tmux: unmanage`, `tmux: send`, `tmux: keys`, or `tmux: follow` is sent without arguments, the bot prompts in-thread and waits for the next reply in that same Slack thread.

## Operational Notes

- Slack replies are posted in a thread rooted at the triggering message
- plain text is routed to the current pane only in DMs or managed threads; channel mainline chatter is never treated as pane input
- plain text stays raw in the default `type` mode; set `plain_text_mode = "execute"` to make bare text send and press Enter, and keep `tmux: send <text>` for raw input when execute mode is enabled
- `tmux: enter <text>` sends text and presses Enter in one step
- use `tmux: keys ...` for tmux key names such as `C-c`, `Enter`, `Escape`, arrows, `PageUp`, `F1`-`F12`, `C-a`-`C-z`, or `M-x`
- `tmux: snapshot` defaults to `image`; `tmux: snapshot text` forces plain text output
- follow mode is one active subscription per Slack conversation tracked by the daemon
- current pane bindings and reply continuity survive daemon restart through SQLite
- follow subscriptions do not survive daemon restart
