# Phase 4 Handoff Context

> Purpose: give the next Codex session enough context to continue from the Phase 3 foundation slice without re-opening already settled architecture decisions.
>
> Last updated: 2026-03-15

## 1. Current Reality

The repo has delivered:

- Phase 1 local tmux bridge
- local HTTP API
- Phase 2 Telegram relay daemon
- Phase 3 foundation for durable Telegram reply continuity

The new persistence baseline now includes:

- `chat_bindings`
- `chat_state`
- `message_log`
- `sessions`
- `message_links`

The daemon now persists enough state to keep Telegram reply targets stable across restart, but it is still relay-first and text-first.

Before implementing anything new, read:

1. [product-phase2.md](./product-phase2.md)
2. [architecture-phase2.md](./architecture-phase2.md)
3. [phase2-telegram-daemon-spec.md](./phase2-telegram-daemon-spec.md)
4. [phase3-status.md](./phase3-status.md)
5. [phase4-todo.md](./phase4-todo.md)

## 2. Non-Negotiable Direction

The project is still **tmux-first**, not agent-runtime-first.

Keep these invariants:

- tmux remains authoritative for pane existence and managed metadata
- daemon/platform session state is secondary control metadata
- relay mode must continue to work when no structured adapter exists
- a daemon restart must not orphan a managed pane
- pane identity must continue to resolve from tmux target, not from Telegram chat/session ids

Still avoid these `cc-connect` drifts:

- do not make a Telegram chat the primary runtime identity of a pane
- do not introduce a large plugin/registry architecture before Telegram structured mode proves out
- do not couple usefulness to a long-lived child-process session handle
- do not replace the existing `tagb.Service` bridge surface with an agent-owned runtime abstraction

## 3. What Just Landed

The last implementation slice added:

- lightweight schema migration/version support using `PRAGMA user_version`
- durable `sessions` and `message_links` tables
- `telegram.SendOptions{ReplyToMessageID}`
- reply continuity in `ReplyBus` based on the latest inbound pane-scoped message
- router/follow integration so pane-scoped replies stay attached to the triggering Telegram message

Important code paths:

- [store.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store.go)
- [messenger.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/messenger.go)
- [router.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router.go)
- [follow.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/follow.go)
- [client.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client.go)

Tests that describe the new baseline:

- [store_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store_test.go)
- [router_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router_test.go)
- [client_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client_test.go)

## 4. Current Limits You Must Remember

These remain real implementation limits:

- there is still no structured adapter boundary in code
- `agent_session_id` and `agent_thread_id` are persisted but unused
- Telegram is still plain text only
- there is still no callback button flow
- there is no outbound message editing
- follow subscriptions are still memory-only and are not restored after restart
- SQLite access still shells out to `sqlite3`
- daemon runtime is still foreground-only; no `launchd` / `systemd` helper yet

## 5. Recommended Next Milestone

The safest next milestone is:

1. keep Telegram as the only platform
2. add a minimal structured adapter boundary as an optional layer
3. start with Codex as the first structured target
4. use the existing `sessions` / `message_links` foundation to power inline actions only after the adapter emits stable higher-level events

Recommended first implementation slice:

- introduce an additive structured adapter interface in `internal/daemon` or a nearby package
- wire adapter selection off tmux metadata agent type, starting with `codex`
- let follow output flow through the adapter when available, while preserving plain relay fallback
- emit a small set of higher-level events needed for approval/continue/stop style controls

Recommended order:

1. structured adapter boundary and event model
2. Codex adapter implementation for the most stable machine-readable output available in practice
3. fallback-to-relay tests for unsupported panes and parser failure
4. Telegram inline action support built on `message_links`
5. mixed structured + raw follow formatting improvements

## 6. What To Borrow From cc-connect

Reference project path:

- `/Users/portgle/code-x/aihub/cc-connect/`

Patterns worth borrowing:

- Telegram callback and reply-context ideas from `platform/telegram/telegram.go`
- lightweight event interpretation patterns, but not the full `core/` architecture
- session/thread persistence ideas only where they fit the tmux-first model

Still do **not** copy directly:

- the large `core/` registry and engine architecture
- abstractions that assume the bridge owns the agent lifecycle
- broad multi-platform generalization before Telegram structured mode is stable

## 7. Suggested Structured-Adapter Shape

Prefer an additive boundary like:

- `relay` remains the default behavior
- `structured adapter` is optional per pane/agent
- adapter consumes pane output and emits higher-level events when it can
- adapter failure degrades to plain relay instead of breaking control

That means:

- do not make `/snapshot`, `/send`, `/enter`, `/ctrlc`, or `/follow` depend on structured parsing
- do not remove raw output relay paths
- do not let adapter state become the source of truth for pane existence

## 8. Acceptance Criteria For The Next Stage

The next stage is on the right track only if all of these remain true:

- an unmanaged or unsupported pane still works in plain relay mode
- Telegram control still works when structured parsing is disabled or fails
- structured-path tests explicitly cover fallback-to-relay behavior
- reply continuity continues to work after daemon restart
- pane identity and recoverability still come from tmux metadata

## 9. Practical Start Checklist For The Next Codex Session

1. Read the docs listed in section 1
2. Read the files listed in section 3
3. Run `go test ./...`
4. Confirm the smallest structured slice before editing
5. If referencing `cc-connect`, borrow patterns instead of architecture

## 10. If Time Is Limited

If the next session can only do one meaningful slice, do this:

- add the minimal Codex structured adapter boundary
- make adapter failure fall back to plain relay
- do not start Feishu/Lark yet
- do not start daemon installer work yet
- do not start bridge-owned pane creation yet

That slice is the narrowest path from today’s persistence/reply foundation to Telegram approval/continue/stop actions.
