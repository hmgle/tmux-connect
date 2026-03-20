# tmux-connect

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![Discord](https://img.shields.io/badge/Discord-Slash%20Commands-5865F2?style=flat-square&logo=discord)](https://discord.com/developers/docs/interactions/application-commands)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README.zh-CN](https://img.shields.io/badge/README-%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-0F766E?style=flat-square)](./README.zh-CN.md)

English | [简体中文](./README.zh-CN.md)

`tmux-connect` is a tmux-first relay for existing panes. It lets you inspect output, send input, expose a local HTTP API, and control a selected pane from Telegram, Slack, or Discord without taking ownership of the pane lifecycle.

Current scope:

- local CLI for attach, inspect, snapshot, stream, and input
- local HTTP control plane over the same bridge service
- multi-connector daemon with Telegram long polling, Slack Socket Mode, and Discord gateway events
- relay-first behavior only; there is no structured Codex/Claude/Gemini protocol yet

## Requirements

- Go `1.25` or later
- `tmux`
- `sqlite3` in `PATH` if you want to run `tmux-connect daemon`
- a Telegram bot token, Slack bot/app tokens, or a Discord bot token if you want remote control

## Build

```bash
go build ./cmd/tmux-connect
```

The repository name is `tmux-connect`; the binary name is `tmux-connect`.

## CLI Quick Start

Configuration can be loaded from `--config PATH` or, by default,
`$XDG_CONFIG_HOME/tmux-connect/config.toml` (falling back to
`$HOME/.config/tmux-connect/config.toml`). Command-line flags override
environment variables, and environment variables override the TOML file.
Global flags such as `--config`, `--socket`, and `--json` must appear
before the subcommand.

List panes:

```bash
go run ./cmd/tmux-connect list
```

Attach an existing pane:

```bash
go run ./cmd/tmux-connect attach --pane %5 --agent codex --label backend
```

Inspect bridge metadata:

```bash
go run ./cmd/tmux-connect inspect --pane %5
```

Send text and press Enter:

```bash
go run ./cmd/tmux-connect send --pane %5 --text "continue" --enter
```

Capture recent output:

```bash
go run ./cmd/tmux-connect snapshot --pane %5 --lines 120
```

Follow output:

```bash
go run ./cmd/tmux-connect stream --pane %5
```

The local CLI surface is:

```bash
tmux-connect [--config PATH] [--socket NAME] [--json] list
tmux-connect [--config PATH] [--socket NAME] [--json] attach --pane %5 [--agent unknown] [--label NAME]
tmux-connect [--config PATH] [--socket NAME] [--json] detach --pane %5
tmux-connect [--config PATH] [--socket NAME] [--json] inspect --pane %5
tmux-connect [--config PATH] [--socket NAME] [--json] snapshot --pane %5 [--lines 120]
tmux-connect [--config PATH] [--socket NAME] [--json] stream --pane %5 [--lines 120]
tmux-connect [--config PATH] [--socket NAME] [--json] send --pane %5 --text "hello" [--enter]
tmux-connect [--config PATH] [--socket NAME] [--json] enter --pane %5
tmux-connect [--config PATH] [--socket NAME] [--json] ctrl-c --pane %5
tmux-connect [--config PATH] [--socket NAME] serve [--listen 127.0.0.1:8080]
tmux-connect [--config PATH] [--socket NAME] daemon <run|doctor|status> [flags]
```

## HTTP API

Start the server:

```bash
go run ./cmd/tmux-connect serve --listen 127.0.0.1:8080
```

Endpoints:

- `GET /healthz`
- `GET /v1/panes`
- `POST /v1/panes/attach`
- `POST /v1/panes/detach`
- `GET /v1/panes/inspect?pane=%250`
- `GET /v1/panes/snapshot?pane=%250&lines=120`
- `POST /v1/panes/send`
- `POST /v1/panes/enter`
- `POST /v1/panes/ctrl-c`
- `GET /v1/panes/stream?pane=%250&lines=120` as SSE

Example:

```bash
curl http://127.0.0.1:8080/v1/panes
curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"continue","enter":true}'
```

## Remote Daemon

Example config file:

```toml
[tmux]
socket = "work"

[serve]
listen = "127.0.0.1:8080"

[daemon]
platform = "telegram"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["123456789"]
poll_timeout = "20s"
snapshot_lines = 120
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
```

Start the Telegram daemon:

```bash
go run ./cmd/tmux-connect daemon run \
  --platform telegram \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

Start the Slack daemon:

```bash
go run ./cmd/tmux-connect daemon run \
  --platform slack \
  --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" \
  --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db
```

Start the Discord daemon:

```bash
go run ./cmd/tmux-connect daemon run \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN" \
  --discord-command-prefix "tmux:" \
  --db ~/.tmux-connect/tmux-connect.db
```

For Slack snapshot images, give the bot `files:write` in addition to the message scopes and reinstall the app after changing scopes.
For Discord prefix commands in channels or DMs, enable the Message Content intent in the developer portal.

Common flags:

- `--platform telegram|slack|discord`
- `--telegram-token TOKEN`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
- `--discord-token TOKEN`
- `--discord-command-prefix PREFIX`
- `--db PATH`
- `--allow-chat CHAT_ID`
- `--poll-timeout 20s`
- `--snapshot-lines 120`
- `--telegram-snapshot-theme dark|light`
- `--telegram-snapshot-font-size 14`
- `--telegram-snapshot-font-file /path/to/font.ttf`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`
- `--telegram-api-base URL`

Checks:

```bash
go run ./cmd/tmux-connect daemon doctor --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
go run ./cmd/tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

Telegram commands:

- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/keys <key...>`
- `/enter [text]`
- `/ctrlc` or `/ctrl-c`
- `/follow on [interval]|off`

Slack commands:

- `tmux: panes`
- `tmux: select <pane>`
- `tmux: clear`
- `tmux: unmanage <pane>`
- `tmux: current`
- `tmux: snapshot [lines] [image|text]`
- `tmux: send <text>`
- `tmux: keys <key...>`
- `tmux: enter [text]`
- `tmux: ctrlc` or `tmux: ctrl-c`
- `tmux: follow on [interval]|off`

Discord commands:

- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/keys <key...>`
- `/enter [text]`
- `/ctrlc` or `/ctrl-c`
- `/follow on [interval]|off`

In Slack channels, start with an app mention such as `@tmux-connect panes`. In Slack DMs and bot-managed threads, use the `tmux:` prefix; Slash-prefixed forms may be intercepted by Slack before they reach the bot. Plain text in Telegram and in Slack DMs or managed threads is sent to the current pane without pressing Enter. Use `/enter <text>` or `tmux: enter <text>` to append Enter, and `/keys` or `tmux: keys` for tmux key names such as `C-c`, `PageUp`, `F1`, or `M-x`. `tmux: snapshot` defaults to `image`. Telegram snapshot images use the built-in `gomono` font, `14` pt, and the `dark` theme by default.
In Discord, slash commands are the primary control surface. Prefixed forms such as `tmux: panes` also work in channels. Plain text is treated as pane input only in DMs; channel replies should keep using slash or prefixed commands.

## Recovery Model

tmux remains the source of truth for pane identity and management metadata. The bridge writes recovery state onto the pane with tmux user options:

- `@tmuxconn_managed=1`
- `@tmuxconn_mode=relay`
- `@tmuxconn_agent=<agent>`
- `@tmuxconn_label=<label>`
- `@tmuxconn_created_by=manual-attach`
- `@tmuxconn_last_activity_unix=<unix timestamp>`

The remote daemon stores platform chat state in SQLite, including bindings, current pane selection, sessions, and message links.

## Docs

- [docs/README.md](./docs/README.md) for the documentation index
- [docs/guide-zh.md](./docs/guide-zh.md) for the Chinese quick start
- [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) for Chinese troubleshooting
- [docs/telegram.md](./docs/telegram.md) for Telegram setup and operations
- [docs/slack.md](./docs/slack.md) for Slack setup and operations
- [docs/discord.md](./docs/discord.md) for Discord setup and operations
- [docs/architecture.md](./docs/architecture.md) for the current system architecture
- [docs/roadmap.md](./docs/roadmap.md) for near-term roadmap items

## Current Limits

- the project is relay-first; there is still no structured agent event parsing
- control keys are limited to `Enter` and `Ctrl-C`
- follow restore does not survive daemon restart yet
- the SQLite layer still shells out to `sqlite3` rather than using an embedded driver

## Acknowledgements

This project was inspired by `cc-connect`, but follows a different direction: a tmux-first relay centered on existing panes rather than a child-process-owned runtime.
