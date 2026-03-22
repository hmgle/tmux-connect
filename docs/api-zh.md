# HTTP API 参考

## 启动服务

```bash
./tmux-connect serve --listen 127.0.0.1:8080
```

## 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 健康检查 |
| GET | `/v1/panes` | 列出所有 pane |
| POST | `/v1/panes/attach` | 接入 pane |
| POST | `/v1/panes/detach` | 分离 pane |
| GET | `/v1/panes/inspect?pane=ID` | 查看 pane 详情 |
| GET | `/v1/panes/snapshot?pane=ID&lines=N` | 抓取输出 |
| POST | `/v1/panes/send` | 发送文本 |
| POST | `/v1/panes/enter` | 发送回车键 |
| POST | `/v1/panes/ctrl-c` | 发送 Ctrl+C |
| GET | `/v1/panes/stream?pane=ID&lines=N` | 流式输出（SSE） |

## 示例

```bash
# 列出 pane
curl http://127.0.0.1:8080/v1/panes

# 发送文本并按回车
curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"make test","enter":true}'

# 获取快照
curl 'http://127.0.0.1:8080/v1/panes/snapshot?pane=%255&lines=80'

# 流式输出
curl -N 'http://127.0.0.1:8080/v1/panes/stream?pane=%255'
```

## 请求 / 响应参考

### GET /healthz

```json
// 响应
{"ok": true, "time": "2025-01-15T10:30:00Z"}
```

### GET /v1/panes

```json
// 响应
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
// 请求
{"pane": "%5", "agent": "codex", "label": "backend"}

// 响应 — 单个 PaneRecord（与 GET /v1/panes 中元素结构相同）
{"info": { ... }, "metadata": { ... }}
```

### POST /v1/panes/detach

```json
// 请求
{"pane": "%5"}

// 响应
{"detached": true, "pane": "%5"}
```

### GET /v1/panes/inspect?pane=%255

```json
// 响应 — 单个 PaneRecord
{"info": { ... }, "metadata": { ... }}
```

### GET /v1/panes/snapshot?pane=%255&lines=120

```json
// 响应
{"pane": "%5", "lines": 120, "snapshot": "$ make test\nok\n..."}
```

### POST /v1/panes/send

```json
// 请求
{"pane": "%5", "text": "continue", "enter": true}

// 响应
{"sent": true, "pane": "%5", "enter": true}
```

### POST /v1/panes/enter

```json
// 请求
{"pane": "%5"}

// 响应
{"sent": true, "pane": "%5", "key": "Enter"}
```

### POST /v1/panes/ctrl-c

```json
// 请求
{"pane": "%5"}

// 响应
{"sent": true, "pane": "%5", "key": "C-c"}
```

### GET /v1/panes/stream?pane=%255&lines=120

Server-Sent Events 流。响应头：`Content-Type: text/event-stream`。

**事件类型：**

```
event: initial
data: {"pane":"%5","lines":120,"content":"$ make test\nok\n..."}

event: output
data: {"pane":"%5","content":"new line\n","at":"2025-01-15T10:30:05Z"}

event: error
data: {"error":"pane not found"}

:keepalive
```

心跳注释每 20 秒发送一次。

## 错误响应

所有端点在出错时返回 JSON 格式错误及对应 HTTP 状态码：

| 状态码 | 含义 |
|--------|------|
| 400 | JSON 格式错误或缺少必填参数 |
| 404 | Pane 未找到 |
| 500 | 内部服务器错误 |
| 502 | tmux 通信错误 |

注意：`lines` 等数值型 query 参数如果无效，会静默回退到默认值，不会返回 400。

```json
{"error": "pane %99 not found"}
```
