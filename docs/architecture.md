# Architecture

## Overview

`tmux-connect` is a tmux-first relay for existing panes.

It exposes three control surfaces over the same bridge behavior:

- local CLI
- local HTTP API
- remote relay daemon

The core design choice is that tmux remains the source of truth for pane identity and pane-level management metadata. The bridge does not own the target process lifecycle.

## Main Components

### `cmd/tmux-connect`

- CLI entrypoint
- global flag parsing
- HTTP server bootstrap
- daemon command dispatch

### `internal/tmux-connect`

- bridge service for pane list, attach, detach, inspect, snapshot, send, and stream
- CLI rendering and exit-code handling

### `internal/tmux`

- tmux command execution
- pane lookup and metadata read/write
- pane snapshot capture
- output streaming through tmux control mode with polling fallback

### `internal/httpapi`

- local HTTP translation layer over the bridge service
- SSE stream endpoint for pane output

### `internal/daemon`

- daemon CLI and config parsing
- pane registry cache
- platform-neutral command router
- follow manager for streaming updates back to chats
- SQLite-backed store for platform chat state and reply continuity

### `internal/telegram`

- Telegram Bot API client
- long polling via `getUpdates`
- outbound message send operations

### `internal/slack`

- Slack Socket Mode client wrapper
- outbound Slack message and image upload operations

### `internal/discord`

- Discord gateway client wrapper using `discordgo`
- slash command registration and interaction handling
- outbound message, embed, and file upload operations

### `internal/whatsapp`

- WhatsApp multi-device client wrapper based on `whatsmeow`
- local session database for paired device state
- QR-based first-time login and outbound text/image send operations

### `internal/termrender`

- render terminal snapshots as PNG images for Telegram delivery

## Runtime Model

### Local CLI / HTTP

1. A caller selects an existing tmux pane.
2. The bridge resolves the pane through tmux.
3. Operations run directly against that pane.
4. If a pane is attached, management metadata is stored on the pane via tmux user options.

### Remote Daemon

1. `tmux-connect daemon run` starts with a connector config and SQLite path.
2. The daemon opens the SQLite store and refreshes pane inventory from tmux.
3. The platform connector starts: Telegram drains pending updates and enters `getUpdates` long polling; Slack opens a Socket Mode connection; Discord opens a gateway connection and registers slash commands; WhatsApp connects with a persisted multi-device session or prints a QR code for first-time pairing.
4. Incoming platform messages are parsed into commands.
5. Commands are routed to the current pane or an explicit pane target.
6. Replies are sent back through the originating platform.
7. Follow mode keeps a tmux subscription open and pushes aggregated output back to the chat.

## Persistence And Recovery

Recovery is split across tmux and SQLite.

### tmux metadata

tmux stores pane-oriented bridge metadata:

- managed flag
- mode
- agent label
- human label
- creation source
- last activity timestamp

This lets the bridge rediscover managed panes after process restart.

### SQLite state

SQLite stores chat-oriented relay state:

- `chat_bindings`
- `chat_state`
- `message_log`
- `sessions`
- `message_links`

This lets the daemon restore current-pane bindings and platform reply continuity across restart.

## Design Principles

- tmux-first: pane identity comes from tmux, not from platform chat/session IDs
- existing-pane-first: the bridge attaches to panes that already exist
- relay-first: plain terminal relay is the baseline behavior
- graceful degradation: stream handling prefers control mode and falls back to polling
- connector-specific transport, shared daemon behavior

## Current Limits

- there is no structured Codex/Claude/Gemini event parsing yet
- remote interaction is still command-driven
- follow subscriptions are not restored automatically after daemon restart
- the CLI's dedicated control-key commands are `enter` and `ctrl-c`; the daemon's `/keys` command supports a wider range of tmux key names (`C-c`, `PageUp`, `F1`, `M-x`, etc.)
- WhatsApp v1 supports only one-to-one chats from another account; self-chat and group chats are ignored
