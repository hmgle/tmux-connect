# tmux-connect

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README.zh-CN](https://img.shields.io/badge/README-%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-0F766E?style=flat-square)](./README.zh-CN.md)

English | [简体中文](./README.zh-CN.md)

`tmux-connect` is a tmux-first relay for existing panes. It lets you inspect output, send input, expose a local HTTP API, and control a selected pane from Telegram or Slack without taking ownership of the pane lifecycle.

Current scope:

- local CLI for attach, inspect, snapshot, stream, and input
- local HTTP control plane over the same bridge service
- multi-connector daemon with Telegram long polling and Slack Socket Mode
- relay-first behavior only; there is no structured Codex/Claude/Gemini protocol yet

## Requirements

- Go `1.25` or later
- `tmux`
- `sqlite3` in `PATH` if you want to run `tagb daemon`
- a Telegram bot token or Slack bot/app tokens if you want remote control

## Build

```bash
go build ./cmd/tagb
```

The repository name is `tmux-connect`; the binary name is `tagb`.

## CLI Quick Start

List panes:

```bash
go run ./cmd/tagb list
```

Attach an existing pane:

```bash
go run ./cmd/tagb attach --pane %5 --agent codex --label backend
```

Inspect bridge metadata:

```bash
go run ./cmd/tagb inspect --pane %5
```

Send text and press Enter:

```bash
go run ./cmd/tagb send --pane %5 --text "continue" --enter
```

Capture recent output:

```bash
go run ./cmd/tagb snapshot --pane %5 --lines 120
```

Follow output:

```bash
go run ./cmd/tagb stream --pane %5
```

The local CLI surface is:

```bash
tagb [--socket NAME] [--json] list
tagb [--socket NAME] [--json] attach --pane %5 [--agent unknown] [--label NAME]
tagb [--socket NAME] [--json] detach --pane %5
tagb [--socket NAME] [--json] inspect --pane %5
tagb [--socket NAME] [--json] snapshot --pane %5 [--lines 120]
tagb [--socket NAME] [--json] stream --pane %5 [--lines 120]
tagb [--socket NAME] [--json] send --pane %5 --text "hello" [--enter]
tagb [--socket NAME] [--json] enter --pane %5
tagb [--socket NAME] [--json] ctrl-c --pane %5
tagb [--socket NAME] serve [--listen 127.0.0.1:8080]
tagb [--socket NAME] daemon <run|doctor|status> [flags]
```

## HTTP API

Start the server:

```bash
go run ./cmd/tagb serve --listen 127.0.0.1:8080
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

Start the Telegram daemon:

```bash
go run ./cmd/tagb daemon run \
  --platform telegram \
  --telegram-token "$TAGB_TELEGRAM_TOKEN" \
  --db ~/.tagb/tagb.db \
  --allow-chat 123456789
```

Start the Slack daemon:

```bash
go run ./cmd/tagb daemon run \
  --platform slack \
  --slack-bot-token "$TAGB_SLACK_BOT_TOKEN" \
  --slack-app-token "$TAGB_SLACK_APP_TOKEN" \
  --db ~/.tagb/tagb.db
```

For Slack snapshot images, give the bot `files:write` in addition to the message scopes and reinstall the app after changing scopes.

Common flags:

- `--platform telegram|slack`
- `--telegram-token TOKEN`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
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
go run ./cmd/tagb daemon doctor --platform telegram --telegram-token "$TAGB_TELEGRAM_TOKEN"
go run ./cmd/tagb daemon status --db ~/.tagb/tagb.db
```

Telegram commands:

- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/enter`
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
- `tmux: enter`
- `tmux: ctrlc` or `tmux: ctrl-c`
- `tmux: follow on [interval]|off`

In Slack channels, start with an app mention such as `@tagb panes`. In Slack DMs and bot-managed threads, use the `tmux:` prefix; Slash-prefixed forms may be intercepted by Slack before they reach the bot. `tmux: snapshot` defaults to `image`. Telegram snapshot images use the built-in `gomono` font, `14` pt, and the `dark` theme by default.

## Recovery Model

tmux remains the source of truth for pane identity and management metadata. The bridge writes recovery state onto the pane with tmux user options:

- `@tagb_managed=1`
- `@tagb_mode=relay`
- `@tagb_agent=<agent>`
- `@tagb_label=<label>`
- `@tagb_created_by=manual-attach`
- `@tagb_last_activity_unix=<unix timestamp>`

The remote daemon stores platform chat state in SQLite, including bindings, current pane selection, sessions, and message links.

## Docs

- [docs/README.md](./docs/README.md) for the documentation index
- [docs/guide-zh.md](./docs/guide-zh.md) for the Chinese quick start
- [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) for Chinese troubleshooting
- [docs/telegram.md](./docs/telegram.md) for Telegram setup and operations
- [docs/slack.md](./docs/slack.md) for Slack setup and operations
- [docs/architecture.md](./docs/architecture.md) for the current system architecture
- [docs/roadmap.md](./docs/roadmap.md) for near-term roadmap items

## Current Limits

- the project is relay-first; there is still no structured agent event parsing
- control keys are limited to `Enter` and `Ctrl-C`
- follow restore does not survive daemon restart yet
- the SQLite layer still shells out to `sqlite3` rather than using an embedded driver

## Acknowledgements

This project was inspired by `cc-connect`, but follows a different direction: a tmux-first relay centered on existing panes rather than a child-process-owned runtime.
