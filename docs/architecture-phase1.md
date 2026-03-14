# Architecture: Phase 1

## Summary

Phase 1 is a single-process local CLI. tmux is the only source of runtime truth. There is no daemon, no database, and no external connector yet.

## Modules

### `cmd/tagb`

CLI entrypoint, subcommand parsing, exit codes, and text or JSON rendering.

### `internal/tagb`

Shared bridge constants and types:

- pane target parsing
- metadata key definitions
- typed exit errors

### `internal/tmux`

Thin tmux execution layer:

- list panes
- capture pane history
- inject text
- send control keys
- read and write pane user options
- follow output via tmux control mode, with a polling fallback

## Runtime Model

1. CLI resolves a pane target from `%5` or `socket:%5`
2. tmux commands operate directly on that pane
3. `attach` writes `@tagb_*` metadata onto the pane
4. `list` and `inspect` read metadata back from tmux
5. `stream` captures an initial snapshot, then follows later changes through control mode or polling fallback

## Recovery Model

Recovery in Phase 1 is metadata recovery, not process recovery:

- tmux panes outlive the CLI
- attached metadata remains on the pane
- a later CLI invocation can rediscover managed panes via `list` and `inspect`

There is no separate registry to reconcile.

## Important Design Choices

- tmux user options are the only persistence layer
- relay mode is the only mode
- text input is pasted literally into the pane
- control input is limited to `Enter` and `Ctrl-C`
- output streaming favors robustness over terminal-perfect fidelity
