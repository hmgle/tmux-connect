# Phase 3 Handoff Context

> Purpose: give the next Codex session enough context to continue implementation quickly without drifting toward the wrong architecture.
>
> Last updated: 2026-03-15
>
> Historical note: this handoff was consumed by the first Phase 3 foundation slice.
>
> Current follow-up docs:
>
> - [phase3-status.md](./phase3-status.md)
> - [phase4-handoff.md](./phase4-handoff.md)
> - [phase4-todo.md](./phase4-todo.md)

## 1. Current Reality

The repo is no longer Phase 1 only.

Delivered baseline:

- Phase 1 local tmux bridge
- local HTTP API
- Phase 2 Telegram relay daemon
- SQLite-backed chat binding / current pane / message log
- command-style Telegram control and follow output push

Recent commits:

- `1f8508f` `docs: sync phase 2 status and roadmap`
- `edad2b3` `feat: add telegram relay daemon`
- `b1108e3` `Optimize tmux metadata and stream handling`
- `0e1985d` `feat: add local http control api`

Before implementing anything new, read:

1. [product-phase2.md](./product-phase2.md)
2. [architecture-phase2.md](./architecture-phase2.md)
3. [phase2-telegram-daemon-spec.md](./phase2-telegram-daemon-spec.md)
4. [phase2-status.md](./phase2-status.md)
5. [phase3-todo.md](./phase3-todo.md)

## 2. Non-Negotiable Direction

The project is **tmux-first**, not agent-process-first.

Keep these invariants:

- tmux is the source of truth for pane existence and managed metadata
- the bridge may restart while panes keep running
- app/platform sessions are control surfaces, not the primary runtime identity
- relay mode must remain usable even when no structured agent adapter exists

Do not drift into these `cc-connect` assumptions:

- do not make the platform chat/session the primary identity of a running pane
- do not require a long-lived child-process handle to keep the bridge useful
- do not force everything behind a large plugin/registry architecture before it is needed
- do not redesign the project around `core.AgentSession`-style abstractions as the first move

## 3. Where The Current Implementation Lives

Core entrypoints:

- [main.go](/Users/portgle/code-x/aihub/tmux-connect-cx/cmd/tagb/main.go)
- [app.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tagb/app.go)

Phase 1 bridge service:

- [service.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tagb/service.go)
- [server.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/httpapi/server.go)

tmux runtime layer:

- [client.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tmux/client.go)
- [control.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tmux/control.go)
- [polling.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tmux/polling.go)
- [metadata.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/tmux/metadata.go)

Phase 2 daemon:

- [cli.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/cli.go)
- [router.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router.go)
- [follow.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/follow.go)
- [registry.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/registry.go)
- [store.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store.go)
- [messenger.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/messenger.go)

Telegram client:

- [client.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client.go)

Tests that describe intended behavior:

- [router_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/router_test.go)
- [store_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/daemon/store_test.go)
- [client_test.go](/Users/portgle/code-x/aihub/tmux-connect-cx/internal/telegram/client_test.go)

## 4. Current Limits You Must Remember

These are real implementation constraints, not just docs:

- Telegram is the only remote connector
- Telegram interaction is plain text only
- there is no callback button flow
- there is no structured agent parsing
- there is no managed pane creation / agent startup flow
- SQLite access is implemented by shelling out to `sqlite3`
- active follow subscriptions are not restored after restart
- message persistence is only a simple log, not durable reply/link mapping
- daemon runtime is foreground-only; no `launchd` / `systemd` helper yet

## 5. Recommended Next Milestone

The safest Phase 3 direction is:

1. stay on Telegram first
2. add agent-aware structured behavior as an optional layer
3. expand persistence only where needed to support that behavior
4. only then consider multi-platform generalization

Recommended first implementation slice:

- add a minimal structured adapter interface for supported agents
- start with one agent that has the most stable machine-readable output in practice
- persist session/thread metadata and message links needed for reply continuity
- add Telegram inline actions only after the message-link layer exists

Recommended order:

1. persistence schema expansion
2. structured adapter boundary
3. Telegram reply/link model
4. approval / continue / stop inline actions
5. follow output upgrades for mixed structured + raw relay mode

## 6. What To Borrow From cc-connect

Reference project path:

- `/Users/portgle/code-x/aihub/cc-connect/`

Useful reference docs:

- `/Users/portgle/code-x/aihub/cc-connect/README.md`
- `/Users/portgle/code-x/aihub/cc-connect/AGENTS.md`
- `/Users/portgle/code-x/aihub/cc-connect/docs/telegram.md`
- `/Users/portgle/code-x/aihub/cc-connect/docs/design-tmux-bridge.md`

Useful code to study:

- Telegram long-poll and update filtering:
  - `/Users/portgle/code-x/aihub/cc-connect/platform/telegram/telegram.go`
- daemon lifecycle and CLI shape:
  - `/Users/portgle/code-x/aihub/cc-connect/cmd/cc-connect/daemon.go`
  - `/Users/portgle/code-x/aihub/cc-connect/daemon/manager.go`
- session and binding persistence patterns:
  - `/Users/portgle/code-x/aihub/cc-connect/core/session.go`
  - `/Users/portgle/code-x/aihub/cc-connect/core/relay.go`
  - `/Users/portgle/code-x/aihub/cc-connect/core/workspace_binding.go`

What is worth borrowing:

- Telegram polling loop shape
- draining stale updates on startup
- command normalization before dispatch
- daemon UX patterns for install/status/logs, when this repo is ready
- message-link and reply-context ideas

What is **not** worth copying directly:

- the large `core/` plugin registry architecture
- agent/session abstractions that assume the bridge owns the agent runtime
- multi-platform generalization before the Telegram + structured-agent path is stable

## 7. Suggested Persistence Evolution

Current tables are intentionally small:

- `chat_bindings`
- `chat_state`
- `message_log`

The next likely schema expansion should add:

- `sessions`
  - logical chat/session key
  - pane key
  - agent session/thread id
  - last inbound and outbound message ids
- `message_links`
  - platform
  - chat id
  - pane key
  - session key
  - inbound message id
  - outbound message id
  - kind

Do not add a `panes` table unless it serves a concrete need beyond what tmux metadata already provides.

## 8. Suggested Structured-Adapter Boundary

If the next session implements structured mode, prefer an additive boundary like:

- `relay` remains the default path
- `structured adapter` is optional per pane/agent
- adapter consumes pane output and emits higher-level events when it can
- adapter failure must degrade back to plain relay, not break control

That means:

- do not replace `tagb.Service` with an agent-owned session model
- do not make follow mode depend on structured parsing
- do not remove raw snapshot/send/control behavior

## 9. Acceptance Criteria For The Next Stage

The next stage is on the right track only if all of these remain true:

- an unmanaged or unsupported pane still works in plain relay mode
- Telegram control still works if structured parsing is disabled
- daemon restart does not orphan managed panes
- pane identity still resolves from tmux target, not chat id
- tests cover both structured-path success and fallback-to-relay behavior

## 10. Practical Start Checklist For The Next Codex Session

1. Read the docs listed in section 1
2. Read the files listed in section 3
3. Run `go test ./...`
4. Decide the smallest Phase 3 slice before editing
5. If referencing `cc-connect`, copy patterns, not architecture

## 11. If Time Is Limited

If the next session can only do one meaningful slice, do this:

- expand persistence to support durable `sessions` + `message_links`
- keep Telegram as the only platform
- do not start Feishu/Lark yet
- do not start daemon installer work yet
- do not start pane-creation orchestration yet

That slice unlocks most of the next product capabilities without forcing an architectural rewrite.
