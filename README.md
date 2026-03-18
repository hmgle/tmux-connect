# tmux-connect

`tmux-connect` is a tmux-first relay for existing panes. It lets you inspect output, send input, expose a local HTTP API, and control a selected pane from Telegram without taking ownership of the pane lifecycle.

Current scope:

- local CLI for attach, inspect, snapshot, stream, and input
- local HTTP control plane over the same bridge service
- Telegram long-polling daemon with SQLite-backed chat bindings and reply continuity
- relay-first behavior only; there is no structured Codex/Claude/Gemini protocol yet

## Requirements

- Go `1.25`
- `tmux`
- `sqlite3` in `PATH` if you want to run `tagb daemon`
- a Telegram bot token from BotFather if you want remote control

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

## Telegram Daemon

Start the daemon:

```bash
go run ./cmd/tagb daemon run \
  --telegram-token "$TAGB_TELEGRAM_TOKEN" \
  --db ~/.tagb/tagb.db \
  --allow-chat 123456789
```

Common flags:

- `--telegram-token TOKEN`
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
go run ./cmd/tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"
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

`/snapshot` defaults to `image`. Telegram snapshot images use the built-in `gomono` font, `14` pt, and the `dark` theme by default.

## Recovery Model

tmux remains the source of truth for pane identity and management metadata. The bridge writes recovery state onto the pane with tmux user options:

- `@tagb_managed=1`
- `@tagb_mode=relay`
- `@tagb_agent=<agent>`
- `@tagb_label=<label>`
- `@tagb_created_by=manual-attach`
- `@tagb_last_activity_unix=<unix timestamp>`

The Telegram daemon stores chat-oriented state in SQLite, including bindings, current pane selection, sessions, and message links.

## Docs

- [docs/README.md](./docs/README.md) for the documentation index
- [docs/guide-zh.md](./docs/guide-zh.md) for the Chinese quick start
- [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) for Chinese troubleshooting
- [docs/telegram.md](./docs/telegram.md) for Telegram setup and operations
- [docs/architecture.md](./docs/architecture.md) for the current system architecture
- [docs/roadmap.md](./docs/roadmap.md) for near-term roadmap items

## Current Limits

- the project is relay-first; there is still no structured agent event parsing
- control keys are limited to `Enter` and `Ctrl-C`
- follow restore does not survive daemon restart yet
- Telegram is the only remote connector today
- the SQLite layer still shells out to `sqlite3` rather than using an embedded driver

## Acknowledgements

This project was inspired by `cc-connect`, but follows a different direction: a tmux-first relay centered on existing panes rather than a child-process-owned runtime.
