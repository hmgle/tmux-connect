# Product Spec: Phase 1 Local tmux Bridge

> Status: completed baseline; retained as historical scope reference. For the current product state, see [product-phase2.md](./product-phase2.md).

## Goal

Provide a local control plane that can run as a CLI or a local HTTP server, attach to an existing tmux pane, and act as a stable relay for:

- reading pane output
- fetching recent pane snapshots
- sending text input
- sending basic control keys
- recovering managed state after the CLI exits

## User

Developers already running Claude Code, Codex CLI, or any other terminal agent inside tmux.

## In Scope

- Local CLI interface
- Local HTTP API for the same bridge operations
- Existing pane attach/detach
- Plain terminal relay mode
- Metadata stored in tmux user options
- Text output snapshots and live follow
- Text input, `Enter`, and `Ctrl-C`

## Out Of Scope

- Telegram, Feishu, or any app integration
- Structured agent parsing
- Permission cards or approval flows
- Bridge-managed pane creation
- SQLite or any local database

## Success Criteria

- A user can discover an existing pane with `list`
- A user can attach the pane and persist bridge metadata
- A user can inspect the pane and see that metadata later
- A user can read recent output with `snapshot`
- A user can follow output with `stream`
- A user can send text and basic control keys to the pane
- A local client can call the same operations through the HTTP API
- Managed state survives CLI restarts because tmux retains metadata

## Non-Goals

- Full terminal emulation
- Shell-aware command execution
- Rich multi-user auth model
- Automatic recovery of arbitrary historical processes without explicit attach
