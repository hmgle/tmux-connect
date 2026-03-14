# CLI Spec: Phase 1

## Root Flags

- `--socket NAME`: operate on a non-default tmux socket
- `--json`: emit machine-readable JSON instead of plain text

## Target Format

Commands accept either:

- `%5`
- `socket:%5`

Internal normalization always uses `<socket>:<pane_id>`. If no socket is provided, the default key is `default:%5`.

## Commands

### `list`

Lists panes visible on the selected socket and shows whether each pane is managed.

### `attach --pane %5 [--agent unknown] [--label NAME]`

Marks an existing pane as managed and writes the Phase 1 metadata set.

### `detach --pane %5`

Removes the Phase 1 metadata keys from a pane.

### `inspect --pane %5`

Shows pane identity, tmux metadata, and bridge metadata.

### `snapshot --pane %5 [--lines 120]`

Prints the recent pane contents captured from tmux history.

### `stream --pane %5 [--lines 120]`

Prints an initial snapshot, then follows later output through tmux control mode. If control mode cannot be established, the CLI falls back to polling tmux snapshots.

### `send --pane %5 --text TEXT [--enter]`

Pastes literal text into the pane. If `--enter` is set, an `Enter` key is sent immediately after the text.

### `enter --pane %5`

Sends the `Enter` key.

### `ctrl-c --pane %5`

Sends the `Ctrl-C` key.

### `serve [--listen 127.0.0.1:8080]`

Starts a local HTTP server that exposes the current relay operations.

## HTTP Endpoints

- `GET /healthz`
- `GET /v1/panes`
- `POST /v1/panes/attach`
- `POST /v1/panes/detach`
- `GET /v1/panes/inspect?pane=%250`
- `GET /v1/panes/snapshot?pane=%250&lines=120`
- `POST /v1/panes/send`
- `POST /v1/panes/enter`
- `POST /v1/panes/ctrl-c`
- `GET /v1/panes/stream?pane=%250&lines=120`

`/v1/panes/stream` returns Server-Sent Events with:

- `event: initial`
- `event: output`
- `event: error`

## Metadata Keys

- `@tagb_managed`
- `@tagb_mode`
- `@tagb_agent`
- `@tagb_label`
- `@tagb_created_by`
- `@tagb_last_activity_unix`

## Exit Codes

- `0`: success
- `2`: invalid input
- `3`: pane target failed
- `4`: tmux command failed

## Error Handling

- unknown commands return exit code `2`
- malformed pane IDs return exit code `2`
- missing panes return exit code `3`
- tmux server failures return exit code `4`

## Current Limits

- only one relay mode exists
- no structured agent protocol support
- no long-lived daemon
- no auth or roles
