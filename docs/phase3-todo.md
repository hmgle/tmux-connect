# Phase 3 TODO

> Historical snapshot before the first Phase 3 foundation slice landed.
>
> Superseded by:
>
> - [phase3-status.md](./phase3-status.md)
> - [phase4-handoff.md](./phase4-handoff.md)
> - [phase4-todo.md](./phase4-todo.md)

## Primary Goal

Move from "remote terminal relay" toward "agent-aware mobile bridge" without breaking the tmux-first runtime model.

## High-Priority TODO

- Introduce a structured adapter layer for Claude Code, Codex CLI, and similar agents where stable machine-readable output exists
- Add durable session/thread metadata beyond the current chat binding model
- Add message-link persistence so permission prompts, follow-up replies, and edit targets can survive restarts
- Add Telegram inline actions for common controls such as continue, stop, and approve/deny
- Improve group-chat behavior with better routing and optional per-user restrictions

## Platform and Runtime TODO

- Add a connector abstraction suitable for Feishu/Lark as the next platform
- Add daemon install/manage workflows for `launchd` and `systemd`
- Add configuration file support instead of env vars and flags only
- Add health and debug surfaces for the daemon beyond current CLI status output

## tmux and Relay TODO

- Support richer control input beyond `Enter` and `Ctrl-C`
- Improve output normalization for prompts, spinners, and repeated repaint-heavy terminal output
- Add optional pane creation / managed start flows for users who want bridge-owned sessions
- Consider a single shared tmux control client for all active follows to reduce overhead

## Data Layer TODO

- Replace shelling out to `sqlite3` with an in-process DB layer
- improve `sessions` / `message_links` so they can support inline callbacks, edit targets, and richer reply threading
- evolve the current minimal migration/versioning path if future schema churn becomes more complex

## Quality TODO

- Add integration tests for daemon polling and recovery flows
- Add fixtures or fakes for pane disappearance and stream errors under follow mode
- Add explicit docs for Telegram setup and operations separate from the README
