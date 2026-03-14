# tmux-connect

`tmux-connect` is a Phase 1 local bridge for controlling existing tmux panes from a CLI.

This first cut is intentionally narrow:

- Go-based local CLI only
- Attach existing panes only
- Relay mode only; no structured Claude/Codex parsing yet
- Recovery via tmux user options only
- No Telegram, Feishu, or HTTP connector yet

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
- No daemon process or platform connectors yet
