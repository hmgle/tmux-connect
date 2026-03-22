# HTTP API Reference

## Start the Server

```bash
./tmux-connect serve --listen 127.0.0.1:8080
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/v1/panes` | List all panes |
| POST | `/v1/panes/attach` | Attach a pane |
| POST | `/v1/panes/detach` | Detach a pane |
| GET | `/v1/panes/inspect?pane=ID` | Inspect a pane |
| GET | `/v1/panes/snapshot?pane=ID&lines=N` | Capture output |
| POST | `/v1/panes/send` | Send text |
| POST | `/v1/panes/enter` | Send Enter key |
| POST | `/v1/panes/ctrl-c` | Send Ctrl+C |
| GET | `/v1/panes/stream?pane=ID&lines=N` | Stream output (SSE) |

## Examples

```bash
# List panes
curl http://127.0.0.1:8080/v1/panes

# Send text with Enter
curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"make test","enter":true}'

# Get snapshot
curl 'http://127.0.0.1:8080/v1/panes/snapshot?pane=%255&lines=80'

# Stream output
curl -N 'http://127.0.0.1:8080/v1/panes/stream?pane=%255'
```

## Request / Response Reference

### GET /healthz

```json
// Response
{"ok": true, "time": "2025-01-15T10:30:00Z"}
```

### GET /v1/panes

```json
// Response
{
  "panes": [
    {
      "info": {
        "target": {"socket": "default", "pane_id": "%1"},
        "session_name": "main",
        "window_id": "@0",
        "window_name": "dev",
        "pane_title": "",
        "current_cmd": "zsh",
        "current_path": "/home/user/project",
        "dead": false,
        "width": 200,
        "height": 50
      },
      "metadata": {
        "managed": true,
        "mode": "relay",
        "agent": "codex",
        "label": "backend",
        "created_by": "manual-attach",
        "last_activity_unix": 1705312200
      }
    }
  ]
}
```

### POST /v1/panes/attach

```json
// Request
{"pane": "%5", "agent": "codex", "label": "backend"}

// Response — single PaneRecord (same shape as items in GET /v1/panes)
{"info": { ... }, "metadata": { ... }}
```

### POST /v1/panes/detach

```json
// Request
{"pane": "%5"}

// Response
{"detached": true, "pane": "%5"}
```

### GET /v1/panes/inspect?pane=%255

```json
// Response — single PaneRecord
{"info": { ... }, "metadata": { ... }}
```

### GET /v1/panes/snapshot?pane=%255&lines=120

```json
// Response
{"pane": "%5", "lines": 120, "snapshot": "$ make test\nok\n..."}
```

### POST /v1/panes/send

```json
// Request
{"pane": "%5", "text": "continue", "enter": true}

// Response
{"sent": true, "pane": "%5", "enter": true}
```

### POST /v1/panes/enter

```json
// Request
{"pane": "%5"}

// Response
{"sent": true, "pane": "%5", "key": "Enter"}
```

### POST /v1/panes/ctrl-c

```json
// Request
{"pane": "%5"}

// Response
{"sent": true, "pane": "%5", "key": "C-c"}
```

### GET /v1/panes/stream?pane=%255&lines=120

Server-Sent Events stream. Headers: `Content-Type: text/event-stream`.

**Event types:**

```
event: initial
data: {"pane":"%5","lines":120,"content":"$ make test\nok\n..."}

event: output
data: {"pane":"%5","content":"new line\n","at":"2025-01-15T10:30:05Z"}

event: error
data: {"error":"pane not found"}

:keepalive
```

Keepalive comments are sent every 20 seconds.

## Error Responses

All endpoints return errors as JSON with the appropriate HTTP status code:

| Status | Meaning |
|--------|---------|
| 400 | Malformed JSON or missing required parameter |
| 404 | Pane not found |
| 500 | Internal server error |
| 502 | Tmux communication error |

Note: numeric query parameters like `lines` silently fall back to their default value when invalid, rather than returning 400.

```json
{"error": "pane %99 not found"}
```
