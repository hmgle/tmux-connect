# Architecture: Phase 1

## Summary

Phase 1 is a single-process local control plane. It can be used as a CLI or exposed through a local HTTP server. tmux is the only source of runtime truth. There is no database and no external connector yet.

## Modules

### `cmd/tagb`

CLI entrypoint, subcommand parsing, exit codes, text or JSON rendering, and local HTTP server bootstrap.

### `internal/httpapi`

HTTP translation layer over the existing bridge service:

- health endpoint
- pane CRUD-like operations for attach and detach
- snapshot and input endpoints
- SSE stream endpoint for pane output

### `internal/tagb`

Bridge service and typed errors:

- pane resolution and attach/detach workflows
- metadata-aware list, inspect, snapshot, input, and stream operations
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

1. A caller reaches the bridge either through the CLI or the local HTTP server
2. The bridge service resolves a pane target from `%5` or `socket:%5`
3. tmux commands operate directly on that pane
4. `attach` writes `@tagb_*` metadata onto the pane
5. `list` and `inspect` read metadata back from tmux
6. `snapshot` returns recent pane history
7. `send`, `enter`, and `ctrl-c` inject literal text or control keys into the pane
8. `stream` captures an initial snapshot, then follows later changes through control mode or polling fallback
9. Over HTTP, the stream endpoint exposes those updates as SSE events

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
- the HTTP server is local-only by deployment convention; auth is deferred
