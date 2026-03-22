# tmux-connect

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![tmux](https://img.shields.io/badge/tmux-required-1BB91F?style=flat-square&logo=tmux)](https://github.com/tmux/tmux)
[![Telegram](https://img.shields.io/badge/Telegram-Bot%20API-26A5E4?style=flat-square&logo=telegram)](https://core.telegram.org/bots/api)
[![Slack](https://img.shields.io/badge/Slack-Socket%20Mode-4A154B?style=flat-square&logo=slack)](https://api.slack.com/apis/connections/socket)
[![Discord](https://img.shields.io/badge/Discord-Slash%20Commands-5865F2?style=flat-square&logo=discord)](https://discord.com/developers/docs/interactions/application-commands)
[![WhatsApp](https://img.shields.io/badge/WhatsApp-whatsmeow-25D366?style=flat-square&logo=whatsapp)](https://pkg.go.dev/go.mau.fi/whatsmeow)
[![License](https://img.shields.io/badge/License-MIT-111827?style=flat-square)](./LICENSE)
[![README](https://img.shields.io/badge/README-English-1F2937?style=flat-square)](./README.md)

[English](./README.md) | 简体中文

随时随地控制你的 tmux pane——CLI、HTTP API，或通过 Telegram、Slack、Discord、WhatsApp 在手机上操作。

## 特性

- **本地 CLI** — 对任意 tmux pane 执行 list、attach、inspect、snapshot、stream 和 send
- **HTTP API** — RESTful 端点 + SSE 流式输出，适合程序化控制
- **聊天中继** — 从 Telegram、Slack、Discord 或 WhatsApp 监控和控制 pane
- **Tmux 优先** — tmux 始终是事实来源；bridge 元数据通过 tmux user options 跨重启保留
- **内嵌 SQLite** — daemon 无需外部数据库

## 快速开始

```bash
# 构建
go build ./cmd/tmux-connect

# 查看所有 pane
./tmux-connect list

# 接入一个 pane 并标记
./tmux-connect attach --pane %5 --label backend

# 发送命令
./tmux-connect send --pane %5 --text "make test" --enter

# 实时跟随输出
./tmux-connect stream --pane %5
```

想在手机上控制同一个 pane？启动 Telegram 中继：

```bash
./tmux-connect daemon run \
  --platform telegram \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

然后在 Telegram 发送 `/panes` 和 `/select %5` 即可开始交互。

## 前提条件

- Go `1.25` 或更高版本
- `tmux`
- 远程控制需要对应平台的 token 或会话（见下方平台指南）

## 文档

| 主题 | 链接 |
|------|------|
| CLI 命令参考 | [docs/cli-zh.md](./docs/cli-zh.md) |
| HTTP API 参考 | [docs/api-zh.md](./docs/api-zh.md) |
| Daemon 配置参考 | [docs/daemon-zh.md](./docs/daemon-zh.md) |
| Telegram 配置 | [docs/telegram.md](./docs/telegram.md) |
| Slack 配置 | [docs/slack.md](./docs/slack.md) |
| Discord 配置 | [docs/discord.md](./docs/discord.md) |
| Discord 接入指南（中文） | [docs/discord-zh.md](./docs/discord-zh.md) |
| WhatsApp 配置 | [docs/whatsapp.md](./docs/whatsapp.md) |
| 系统架构 | [docs/architecture.md](./docs/architecture.md) |
| 路线图 | [docs/roadmap.md](./docs/roadmap.md) |
| 中文快速开始 | [docs/guide-zh.md](./docs/guide-zh.md) |
| 中文故障排查 | [docs/troubleshooting-zh.md](./docs/troubleshooting-zh.md) |

## 当前限制

- 目前仍以 relay-first 为主，还没有结构化 agent 事件解析
- follow 状态不能跨 daemon 重启保留
- WhatsApp v1 仅支持私聊

## 致谢

受 `cc-connect` 启发，但方向不同：围绕现有 pane 的 tmux-first 中继，而非进程级运行时。
