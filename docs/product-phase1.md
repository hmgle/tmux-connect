# Product Spec: Phase 1 Local tmux Bridge

## Goal

Provide a local CLI tool that can attach to an existing tmux pane and act as a stable relay for:

- reading pane output
- fetching recent pane snapshots
- sending text input
- sending basic control keys
- recovering managed state after the CLI exits

## User

Developers already running Claude Code, Codex CLI, or any other terminal agent inside tmux.

## In Scope

- Local CLI interface only
- Existing pane attach/detach
- Plain terminal relay mode
- Metadata stored in tmux user options
- Text output snapshots and live follow
- Text input, `Enter`, and `Ctrl-C`

## Out Of Scope

- Telegram, Feishu, or any app integration
- Local HTTP API
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
- Managed state survives CLI restarts because tmux retains metadata

## Non-Goals

- Full terminal emulation
- Shell-aware command execution
- Rich multi-user auth model
- Automatic recovery of arbitrary historical processes without explicit attach
