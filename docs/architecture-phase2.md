# Architecture: Phase 2

> Status: implemented baseline on 2026-03-15

## Summary

Phase 2 adds a Telegram-facing daemon on top of the Phase 1 tmux bridge. tmux remains the source of truth for pane existence and management metadata. SQLite stores chat-oriented relay state that tmux should not own.

## Modules

### `cmd/tagb`

- existing CLI entrypoint
- local HTTP server bootstrap
- daemon command dispatch: `run`, `doctor`, `status`

### `internal/tagb`

- pane attach/detach/list/inspect/snapshot/send/stream service
- unchanged bridge semantics from Phase 1

### `internal/tmux`

- tmux execution layer
- pane metadata read/write
- snapshot capture
- control-mode stream with polling fallback

### `internal/telegram`

- Telegram Bot API client
- `getUpdates` long polling
- `sendMessage`
- stale update drain on daemon startup

### `internal/daemon`

- daemon CLI/config parsing
- pane registry cache built from `tagb.Service.List`
- SQLite store for chat bindings, current pane, and message log
- Telegram command router
- follow manager with output aggregation and push delivery

## Runtime Model

1. `tagb daemon run` starts with Telegram token and SQLite path
2. daemon opens the SQLite store and refreshes pane inventory from tmux
3. daemon drains stale Telegram updates
4. daemon enters long polling on `getUpdates`
5. each Telegram text message is normalized into a command and routed
6. commands call the existing bridge service against the current or explicit pane
7. replies are sent back through Telegram and logged into SQLite
8. follow mode opens an existing tmux subscription and pushes aggregated output to the bound chat

## Recovery Model

Recovery is split across two layers:

- tmux metadata:
  - managed pane status
  - mode
  - agent
  - label
  - last activity
- SQLite:
  - which chats are bound to which panes
  - the current pane for each chat
  - a simple inbound/outbound message log

On restart, the daemon rebuilds pane availability from tmux and reuses SQLite only for chat-side state.

## Important Design Choices

- tmux stays authoritative for pane existence
- Phase 2 is still relay-only
- Telegram interaction is command-driven rather than conversational
- follow output is aggregated into chunked Telegram messages
- SQLite is accessed through the `sqlite3` CLI for now, not an embedded driver
- allowlisting is optional and chat-scoped
