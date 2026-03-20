# tmux-connect

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README](https://img.shields.io/badge/README-English-1F2937?style=flat-square)](./README.md)

[English](./README.md) | 简体中文

`tmux-connect` 是一个面向现有 tmux pane 的 tmux-first 中继工具。它允许你查看输出、发送输入、暴露本地 HTTP API，并通过 Telegram 或 Slack 控制选定的 pane，而不接管 pane 生命周期。

当前范围：

- 面向 attach、inspect、snapshot、stream 和 input 的本地 CLI
- 基于同一桥接服务的本地 HTTP 控制面
- 同一个 daemon 支持 Telegram 长轮询和 Slack Socket Mode
- 当前仍以 relay-first 为主；尚未提供结构化的 Codex/Claude/Gemini 协议

## Requirements

- Go `1.25` 或更高版本
- `tmux`
- 如果要运行 `tmux-connect daemon`，需要 `PATH` 中可用的 `sqlite3`
- 如果要远程控制，需要 Telegram bot token 或 Slack bot/app token

## Build

```bash
go build ./cmd/tmux-connect
```

仓库名是 `tmux-connect`，生成的二进制名是 `tmux-connect`。

## CLI Quick Start

列出 pane：

```bash
go run ./cmd/tmux-connect list
```

接入一个已存在的 pane：

```bash
go run ./cmd/tmux-connect attach --pane %5 --agent codex --label backend
```

查看 bridge metadata：

```bash
go run ./cmd/tmux-connect inspect --pane %5
```

发送文本并附带回车：

```bash
go run ./cmd/tmux-connect send --pane %5 --text "continue" --enter
```

抓取最近输出：

```bash
go run ./cmd/tmux-connect snapshot --pane %5 --lines 120
```

持续跟随输出：

```bash
go run ./cmd/tmux-connect stream --pane %5
```

本地 CLI 入口如下：

```bash
tmux-connect [--socket NAME] [--json] list
tmux-connect [--socket NAME] [--json] attach --pane %5 [--agent unknown] [--label NAME]
tmux-connect [--socket NAME] [--json] detach --pane %5
tmux-connect [--socket NAME] [--json] inspect --pane %5
tmux-connect [--socket NAME] [--json] snapshot --pane %5 [--lines 120]
tmux-connect [--socket NAME] [--json] stream --pane %5 [--lines 120]
tmux-connect [--socket NAME] [--json] send --pane %5 --text "hello" [--enter]
tmux-connect [--socket NAME] [--json] enter --pane %5
tmux-connect [--socket NAME] [--json] ctrl-c --pane %5
tmux-connect [--socket NAME] serve [--listen 127.0.0.1:8080]
tmux-connect [--socket NAME] daemon <run|doctor|status> [flags]
```

## HTTP API

启动服务：

```bash
go run ./cmd/tmux-connect serve --listen 127.0.0.1:8080
```

端点：

- `GET /healthz`
- `GET /v1/panes`
- `POST /v1/panes/attach`
- `POST /v1/panes/detach`
- `GET /v1/panes/inspect?pane=%250`
- `GET /v1/panes/snapshot?pane=%250&lines=120`
- `POST /v1/panes/send`
- `POST /v1/panes/enter`
- `POST /v1/panes/ctrl-c`
- `GET /v1/panes/stream?pane=%250&lines=120`，通过 SSE 输出

示例：

```bash
curl http://127.0.0.1:8080/v1/panes
curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"continue","enter":true}'
```

## Remote Daemon

启动 Telegram daemon：

```bash
go run ./cmd/tmux-connect daemon run \
  --platform telegram \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

启动 Slack daemon：

```bash
go run ./cmd/tmux-connect daemon run \
  --platform slack \
  --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" \
  --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db
```

如果要让 Slack 的 snapshot 发图片，除了消息相关 scope 之外，还要给 bot `files:write`，并且在变更 scope 后重新安装应用。

常用参数：

- `--platform telegram|slack`
- `--telegram-token TOKEN`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
- `--db PATH`
- `--allow-chat CHAT_ID`
- `--poll-timeout 20s`
- `--snapshot-lines 120`
- `--telegram-snapshot-theme dark|light`
- `--telegram-snapshot-font-size 14`
- `--telegram-snapshot-font-file /path/to/font.ttf`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`
- `--telegram-api-base URL`

检查命令：

```bash
go run ./cmd/tmux-connect daemon doctor --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
go run ./cmd/tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

Telegram 命令：

- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/keys <key...>`
- `/enter [text]`
- `/ctrlc` 或 `/ctrl-c`
- `/follow on [interval]|off`

Slack 命令：

- `tmux: panes`
- `tmux: select <pane>`
- `tmux: clear`
- `tmux: unmanage <pane>`
- `tmux: current`
- `tmux: snapshot [lines] [image|text]`
- `tmux: send <text>`
- `tmux: keys <key...>`
- `tmux: enter [text]`
- `tmux: ctrlc` 或 `tmux: ctrl-c`
- `tmux: follow on [interval]|off`

Slack 频道里建议用 app mention 起命令，例如 `@tmux-connect panes`；Slack 私聊和 bot thread 里用 `tmux:` 作为命令前缀。带 `/` 的写法可能会先被 Slack 当成真正的 Slash Command 拦截，发不到 bot。Telegram 以及 Slack 私聊和受管 thread 里的纯文本会直接发送到当前 pane，但不会自动附带回车；需要执行时，用 `/enter <text>` 或 `tmux: enter <text>`。发送 tmux 特殊键时，使用 `/keys` 或 `tmux: keys`，例如 `C-c`、`PageUp`、`F1`、`M-x`。`tmux: snapshot` 默认使用 `image`。Telegram snapshot 图片默认使用内置 `gomono` 字体、`14` pt 字号和 `dark` 主题。

## Recovery Model

tmux 仍然是 pane 身份和管理元数据的事实来源。bridge 会通过 tmux user options 把恢复状态写回 pane：

- `@tmuxconn_managed=1`
- `@tmuxconn_mode=relay`
- `@tmuxconn_agent=<agent>`
- `@tmuxconn_label=<label>`
- `@tmuxconn_created_by=manual-attach`
- `@tmuxconn_last_activity_unix=<unix timestamp>`

remote daemon 会把平台聊天状态保存在 SQLite 中，包括绑定关系、当前 pane 选择、session 和消息关联。

## Docs

- [docs/README.md](./docs/README.md) 文档索引
- [docs/guide-zh.md](./docs/guide-zh.md) 中文快速开始
- [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) 中文故障排查
- [docs/telegram.md](./docs/telegram.md) Telegram 配置与操作
- [docs/slack.md](./docs/slack.md) Slack 配置与操作
- [docs/architecture.md](./docs/architecture.md) 当前系统架构
- [docs/roadmap.md](./docs/roadmap.md) 近期路线图

## Current Limits

- 项目目前仍以 relay-first 为主，还没有结构化 agent 事件解析
- 控制键目前仅支持 `Enter` 和 `Ctrl-C`
- follow 恢复状态尚不能跨 daemon 重启保留
- SQLite 层仍然通过调用 `sqlite3` 命令，而不是内嵌 driver

## Acknowledgements

这个项目受到了 `cc-connect` 的启发，但方向不同：这里是围绕现有 pane 的 tmux-first relay，而不是围绕子进程生命周期的运行时。
