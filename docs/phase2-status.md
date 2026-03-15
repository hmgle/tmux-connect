# Phase 2 Status

> Snapshot date: 2026-03-15

## Implemented

- `tagb daemon run`, `doctor`, and `status`
- Telegram Bot API long polling client
- stale-update drain at daemon startup
- chat allowlist support
- chat-to-pane binding stored in SQLite
- current-pane tracking stored in SQLite
- simple inbound/outbound message log
- Telegram command router for pane listing, binding, snapshot, send, `Enter`, `Ctrl-C`, and follow
- follow mode using the existing tmux stream layer with message aggregation
- detach cleanup that removes stale bindings and stops follow sessions
- unit tests for Telegram client, SQLite store, router flows, and follow behavior

## Implemented But Intentionally Minimal

- Telegram responses are plain text only
- follow aggregation is time-window based and does not yet do advanced dedupe or semantic summarization
- allowlisting is chat-level only
- SQLite access currently shells out to `sqlite3`
- daemon runtime is foreground-only; there is no OS service installer yet

## Not Implemented Yet

- structured Claude/Codex/Gemini event parsing
- permission request/approval cards
- inline button callbacks
- outbound message editing instead of append-only replies
- multi-platform connector abstraction beyond Telegram
- bridge-created panes or managed agent startup
- richer session/thread recovery tied to agent-native IDs
- DB schema for per-message reply threading and durable outbound linkage

## Optimization Opportunities

- replace `sqlite3` CLI calls with an embedded SQLite driver
- persist and restore active follow subscriptions after restart
- improve snapshot trimming and follow chunk formatting for very noisy terminals
- add better stale-binding reconciliation when panes move across sockets or are recreated
- add lightweight metrics or structured logs for polling, routing, and tmux stream failures
