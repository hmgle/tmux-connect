# tmux-connect

[![CI](https://github.com/hmgle/tmux-connect/actions/workflows/ci.yml/badge.svg)](https://github.com/hmgle/tmux-connect/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![Discord](https://img.shields.io/badge/Discord-Slash%20Commands-5865F2?style=flat-square&logo=discord)](https://discord.com/developers/docs/interactions/application-commands)
[![WhatsApp](https://img.shields.io/badge/WhatsApp-whatsmeow-25D366?style=flat-square&logo=whatsapp)](https://pkg.go.dev/go.mau.fi/whatsmeow)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README.zh-CN](https://img.shields.io/badge/README-%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-0F766E?style=flat-square)](./README.zh-CN.md)

English | [简体中文](./README.zh-CN.md)

Control your tmux panes from anywhere — CLI, HTTP API, or your phone via Telegram, Feishu, Slack, Discord, and WhatsApp.

## Features

- **Local CLI** — list, attach, inspect, snapshot, stream, and send input to any tmux pane
- **HTTP API** — RESTful endpoints with SSE streaming for programmatic control
- **Chat relay** — monitor and control panes from Telegram, Feishu, Slack, Discord, or WhatsApp
- **Tmux-first** — tmux stays the source of truth; bridge metadata survives restarts via tmux user options
- **Embedded SQLite** — no external database required for the daemon

## Quick Start

```bash
# Build
make build

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

## Selective Builds

By default `make build` compiles all supported remote platforms into the binary.
Run `make platforms` first to see the platform names accepted by
`PLATFORMS_INCLUDE` and `EXCLUDE`:

```bash
make platforms
```

Supported platform names:

| Platform | Build name | Credentials / session | Guide |
|----------|------------|-----------------------|-------|
| Telegram | `telegram` | Bot token | [docs/telegram.md](./docs/telegram.md) |
| Feishu | `feishu` | App ID + App Secret | [docs/feishu.md](./docs/feishu.md) |
| Slack | `slack` | Bot token + App token | [docs/slack.md](./docs/slack.md) |
| Discord | `discord` | Bot token | [docs/discord.md](./docs/discord.md) |
| WhatsApp | `whatsapp` | Paired device session DB | [docs/whatsapp.md](./docs/whatsapp.md) |

If you only need a subset, use negative build tags through the `Makefile`:

```bash
# Keep only Telegram and Discord
make build PLATFORMS_INCLUDE=telegram,discord

# Or exclude heavy integrations you do not need
make build EXCLUDE=feishu,whatsapp
```

`EXCLUDE` and `PLATFORMS_INCLUDE` are mutually exclusive, and unknown platform
names fail fast instead of being ignored. `make platforms` accepts the same
arguments, so you can preview the effective selection before building.

After building, `./tmux-connect daemon help` prints the platforms compiled into
that binary, and the `--platform` flag help matches the current build.

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
| Feishu Setup | [docs/feishu.md](./docs/feishu.md) |
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
