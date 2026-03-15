# Phase 3 Status

> Snapshot date: 2026-03-15

## Implemented In The First Phase 3 Slice

- SQLite schema version tracking via `PRAGMA user_version`
- durable `sessions` table for chat + pane continuity
- durable `message_links` table for inbound/outbound Telegram linkage
- idempotent session creation keyed by `telegram:<chat_id>:<pane_key>`
- reply continuity using Telegram `reply_to_message_id`
- session-aware reply handling for `/bind`, `/current`, `/snapshot`, `/send`, `/enter`, `/ctrlc`, and `/follow`
- restart-safe recovery of reply targets from persisted session/message-link state
- tests for migration, session persistence, Telegram reply targets, and follow/snapshot continuity

## Current Behavior

- tmux is still the source of truth for pane identity and managed metadata
- Telegram remains the only remote connector
- plain relay still works for unsupported or unmanaged panes
- non-pane-scoped commands such as `/help`, `/panes`, `/attach`, and `/detach` still use simple non-session replies
- `message_log` remains as a simple audit log alongside the new session/message-link tables

## Implemented But Intentionally Minimal

- `sessions.agent_session_id` and `sessions.agent_thread_id` are schema placeholders only
- reply continuity currently uses the latest inbound Telegram message as the anchor
- reply linkage does not yet support outbound message editing, callback restoration, or deep reply-tree reconstruction
- migrations are intentionally lightweight and still rely on the `sqlite3` CLI

## Not Implemented Yet

- structured Codex / Claude / Gemini parsing
- adapter-driven mixed structured + raw follow output
- Telegram inline buttons and callback handling
- approval / continue / stop actions
- follow subscription restore after daemon restart
- in-process SQLite driver
- multi-platform connector abstraction
- bridge-owned pane creation or managed agent startup

## Files That Matter Most Now

- [store.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store.go)
- [messenger.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/messenger.go)
- [router.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router.go)
- [client.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client.go)
- [store_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store_test.go)
- [router_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router_test.go)
- [client_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client_test.go)
