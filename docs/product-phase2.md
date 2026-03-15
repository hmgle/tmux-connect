# Product Spec: Phase 2 Telegram Relay Daemon

> Status: implemented baseline on 2026-03-15

## Goal

Extend the local tmux bridge so an attached tmux pane can be viewed and controlled from Telegram without changing tmux to a child-process-owned runtime.

## User

Developers already running Claude Code, Codex CLI, Gemini CLI, or other terminal tools inside tmux and who want basic remote control from Telegram.

## In Scope

- Long-running `tagb daemon` process
- Telegram long polling connector
- Command-based Telegram interaction
- Existing pane attach/detach workflows
- Chat-to-pane binding and current-pane tracking
- SQLite-backed relay state for chat bindings and message logs
- Snapshot, send text, `Enter`, `Ctrl-C`, and follow mode from Telegram
- Recovery of managed pane inventory from tmux metadata after daemon restart

## Out Of Scope

- Structured Claude/Codex/Gemini parsing
- Permission cards or approval actions
- Rich Telegram inline keyboards or callback workflows
- Bridge-managed pane creation
- Feishu, Slack, Discord, or other platform connectors
- Multi-user role and permission model inside group chats

## Success Criteria

- A user can start `tagb daemon run` with a Telegram bot token
- A Telegram chat can list panes and bind to an attached pane
- A bound chat can request snapshots and send text/control input
- A bound chat can enable and disable follow mode
- Chat binding and current-pane state survive daemon restart
- If a pane is detached or disappears, the daemon returns a clear error and clears stale current-pane state

## Non-Goals

- Full terminal emulation in Telegram
- Thread-perfect message reconstruction after restart
- Agent-specific session/thread recovery
- Fine-grained access control beyond optional chat allowlisting
