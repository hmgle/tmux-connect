# tmux-connect

`tmux-connect` is a local tmux bridge for controlling existing panes from a CLI, HTTP API, or a Telegram relay daemon.

Current scope:

- Go-based local CLI and daemon
- Attach existing panes only
- Relay mode only; no structured Claude/Codex parsing yet
- Recovery via tmux user options plus a local SQLite state store for Telegram chat bindings, sessions, and message links
- Local HTTP control plane
- Telegram long-polling daemon for remote pane relay
- Telegram reply continuity backed by durable session/message-link persistence

## Commands

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
tagb [--socket NAME] daemon run --telegram-token TOKEN [--db PATH] [--allow-chat 123456]
tagb [--socket NAME] daemon doctor --telegram-token TOKEN [--db PATH]
tagb [--socket NAME] daemon status [--db PATH]
```

## Quick Start

1. Start or reuse a tmux pane.
2. List panes:

```bash
go run ./cmd/tagb list
```

3. Attach a pane to the bridge:

```bash
go run ./cmd/tagb attach --pane %5 --agent claude --label backend
```

4. Inspect bridge metadata:

```bash
go run ./cmd/tagb inspect --pane %5
```

5. Send text and press Enter:

```bash
go run ./cmd/tagb send --pane %5 --text "continue" --enter
```

6. Capture recent output:

```bash
go run ./cmd/tagb snapshot --pane %5 --lines 120
```

7. Follow pane output:

```bash
go run ./cmd/tagb stream --pane %5
```

8. Start the local HTTP API:

```bash
go run ./cmd/tagb serve --listen 127.0.0.1:8080
```

## Telegram Relay Daemon

The daemon keeps a Telegram bot connected and routes command-style chat operations to the currently bound tmux pane.

Requirements:

- `sqlite3` must be installed and available in `PATH`
- a Telegram bot token from BotFather

Start the daemon:

```bash
go run ./cmd/tagb daemon run \
  --telegram-token "$TAGB_TELEGRAM_TOKEN" \
  --db ~/.tagb/tagb.db \
  --telegram-snapshot-theme light \
  --telegram-snapshot-font-size 16 \
  --telegram-snapshot-font-file /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf \
  --allow-chat 123456789
```

Useful checks:

```bash
go run ./cmd/tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"
go run ./cmd/tagb daemon status --db ~/.tagb/tagb.db
```

Supported Telegram commands:

- `/panes`
- `/attach <pane>`
- `/detach <pane>`
- `/bind <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/enter`
- `/ctrlc`
- `/follow on|off`

`/snapshot` defaults to `image`. Use `/snapshot text` when you need the pane content as Telegram text.
Telegram snapshot images default to the built-in `gomono` font, `14` pt, and the `dark` theme. Override them with `--telegram-snapshot-theme`, `--telegram-snapshot-font-size`, or `--telegram-snapshot-font-file` (or the matching `TAGB_TELEGRAM_SNAPSHOT_*` env vars).

Detailed daemon and Telegram docs:

- `docs/README.md`
- `docs/product-phase2.md`
- `docs/architecture-phase2.md`
- `docs/phase2-telegram-daemon-spec.md`
- `docs/phase3-status.md`
- `docs/phase4-handoff.md`
- `docs/telegram.md`

## HTTP API

The local server exposes the same control surface over HTTP:

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

## Metadata

Phase 1 stores recovery state directly on the tmux pane:

- `@tagb_managed=1`
- `@tagb_mode=relay`
- `@tagb_agent=<agent>`
- `@tagb_label=<label>`
- `@tagb_created_by=manual-attach`
- `@tagb_last_activity_unix=<unix timestamp>`

Managed state survives CLI restarts because tmux keeps the pane metadata.

## Exit Codes

- `0`: success
- `2`: invalid user input
- `3`: pane not found or pane operation failed
- `4`: tmux communication failed

## Current Limits

- Streaming prefers tmux control mode and falls back to polling snapshots when control mode is unavailable
- Control keys currently support only `Enter` and `Ctrl-C`
- Multi-socket support is explicit via `--socket`; there is no auto-discovery across sockets
- Telegram is the only remote connector today
- Telegram interaction is command-based; reply continuity exists, but there is still no structured agent protocol, approval card flow, or rich inline UI
- The local state store currently shells out to `sqlite3`; schema versioning now exists, but there is still no embedded DB layer
