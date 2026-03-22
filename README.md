# tmux-connect

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![Discord](https://img.shields.io/badge/Discord-Slash%20Commands-5865F2?style=flat-square&logo=discord)](https://discord.com/developers/docs/interactions/application-commands)
[![WhatsApp](https://img.shields.io/badge/WhatsApp-whatsmeow-25D366?style=flat-square&logo=whatsapp)](https://pkg.go.dev/go.mau.fi/whatsmeow)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README.zh-CN](https://img.shields.io/badge/README-%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-0F766E?style=flat-square)](./README.zh-CN.md)

English | [简体中文](./README.zh-CN.md)

Control your tmux panes from anywhere — CLI, HTTP API, or your phone via Telegram, Slack, Discord, and WhatsApp.

## Features

- **Local CLI** — list, attach, inspect, snapshot, stream, and send input to any tmux pane
- **HTTP API** — RESTful endpoints with SSE streaming for programmatic control
- **Chat relay** — monitor and control panes from Telegram, Slack, Discord, or WhatsApp
- **Tmux-first** — tmux stays the source of truth; bridge metadata survives restarts via tmux user options
- **Embedded SQLite** — no external database required for the daemon

## Quick Start

```bash
# Build
go build ./cmd/tmux-connect

# See your panes
./tmux-connect list

# Attach to a pane and label it
./tmux-connect attach --pane %5 --label backend

# Send a command
./tmux-connect send --pane %5 --text "make test" --enter

# Watch output in real time
./tmux-connect stream --pane %5
```

Want to control the same pane from your phone? Start a Telegram relay:

```bash
./tmux-connect daemon run \
  --platform telegram \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

Then send `/panes` and `/select %5` from Telegram to start interacting.

## Requirements

- Go `1.25` or later
- `tmux`
- A platform token or session for remote control (see platform guides below)

## Documentation

| Topic | Link |
|-------|------|
| CLI Reference | [docs/cli.md](./docs/cli.md) |
| HTTP API Reference | [docs/api.md](./docs/api.md) |
| Daemon Configuration | [docs/daemon.md](./docs/daemon.md) |
| Telegram Setup | [docs/telegram.md](./docs/telegram.md) |
| Slack Setup | [docs/slack.md](./docs/slack.md) |
| Discord Setup | [docs/discord.md](./docs/discord.md) |
| WhatsApp Setup | [docs/whatsapp.md](./docs/whatsapp.md) |
| Architecture | [docs/architecture.md](./docs/architecture.md) |
| Roadmap | [docs/roadmap.md](./docs/roadmap.md) |
| 中文快速开始 | [docs/guide-zh.md](./docs/guide-zh.md) |
| 中文故障排查 | [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) |

## Current Limits

- Relay-first only; no structured agent protocol parsing yet
- Follow state does not survive daemon restart
- WhatsApp v1: private chats only

## Acknowledgements

Inspired by `cc-connect`, but follows a different direction: a tmux-first relay for existing panes rather than a process-owned runtime.
