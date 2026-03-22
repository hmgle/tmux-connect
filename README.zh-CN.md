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

`tmux-connect` 是一个面向现有 tmux pane 的 tmux-first 中继工具。它允许你查看输出、发送输入、暴露本地 HTTP API，并通过 Telegram、Slack、Discord 或 WhatsApp 控制选定的 pane，而不接管 pane 生命周期。

当前范围：

- 面向 attach、inspect、snapshot、stream 和 input 的本地 CLI
- 基于同一桥接服务的本地 HTTP 控制面
- 同一个 daemon 支持 Telegram 长轮询、Slack Socket Mode、Discord gateway 事件和 WhatsApp 多设备登录
- 当前仍以 relay-first 为主；尚未提供结构化的 Codex/Claude/Gemini 协议

## Requirements

- Go `1.25` 或更高版本
- `tmux`
- daemon 使用内嵌 SQLite，不再要求系统额外安装 `sqlite3` 命令行
- 如果要远程控制，需要 Telegram bot token、Slack bot/app token、Discord bot token 或已配对的 WhatsApp 设备会话

## Build

```bash
go build ./cmd/tmux-connect
```

构建后会在仓库根目录生成 `./tmux-connect`。仓库名和二进制名都是 `tmux-connect`。

以下示例使用 `./tmux-connect`。如果不想先编译，可以将 `./tmux-connect` 替换为 `go run ./cmd/tmux-connect`。

## CLI Quick Start

配置可以通过 `--config PATH` 指定，默认读取
`$XDG_CONFIG_HOME/tmux-connect/config.toml`，若未设置则回退到
`$HOME/.config/tmux-connect/config.toml`。优先级是命令行参数 >
环境变量 > TOML 配置文件。像 `--config`、`--socket`、`--json`
这类全局参数需要放在子命令之前。

列出 pane：

```bash
./tmux-connect list
```

接入一个已存在的 pane：

```bash
./tmux-connect attach --pane %5 --agent codex --label backend
```

查看 bridge metadata：

```bash
./tmux-connect inspect --pane %5
```

发送文本并附带回车：

```bash
./tmux-connect send --pane %5 --text "continue" --enter
```

抓取最近输出：

```bash
./tmux-connect snapshot --pane %5 --lines 120
```

持续跟随输出：

```bash
./tmux-connect stream --pane %5
```

本地 CLI 入口如下：

```bash
./tmux-connect [--config PATH] [--socket NAME] [--json] list
./tmux-connect [--config PATH] [--socket NAME] [--json] attach --pane %5 [--agent unknown] [--label NAME]
./tmux-connect [--config PATH] [--socket NAME] [--json] detach --pane %5
./tmux-connect [--config PATH] [--socket NAME] [--json] inspect --pane %5
./tmux-connect [--config PATH] [--socket NAME] [--json] snapshot --pane %5 [--lines 120]
./tmux-connect [--config PATH] [--socket NAME] [--json] stream --pane %5 [--lines 120]
./tmux-connect [--config PATH] [--socket NAME] [--json] send --pane %5 --text "hello" [--enter]
./tmux-connect [--config PATH] [--socket NAME] [--json] enter --pane %5
./tmux-connect [--config PATH] [--socket NAME] [--json] ctrl-c --pane %5
./tmux-connect [--config PATH] [--socket NAME] serve [--listen 127.0.0.1:8080]
./tmux-connect [--config PATH] [--socket NAME] daemon <run|doctor|status> [flags]
```

## HTTP API

启动服务：

```bash
./tmux-connect serve --listen 127.0.0.1:8080
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

daemon 也支持通过 `--config PATH` 或默认
`$XDG_CONFIG_HOME/tmux-connect/config.toml` 读取配置，未设置时会回退到
`$HOME/.config/tmux-connect/config.toml`。命令行参数优先于环境变量，
环境变量优先于 TOML 配置文件。

启动 Telegram daemon：

```bash
./tmux-connect daemon run \
  --platform telegram \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

启动 Slack daemon：

```bash
./tmux-connect daemon run \
  --platform slack \
  --slack-bot-token "$TMUXCONN_SLACK_BOT_TOKEN" \
  --slack-app-token "$TMUXCONN_SLACK_APP_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db
```

启动 Discord daemon：

```bash
./tmux-connect daemon run \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN" \
  --discord-command-prefix "tmux:" \
  --db ~/.tmux-connect/tmux-connect.db
```

启动 WhatsApp daemon：

```bash
./tmux-connect daemon run \
  --platform whatsapp \
  --whatsapp-session-db ~/.tmux-connect/whatsapp-device.db \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat whatsapp:8613800000000@s.whatsapp.net
```

如果要让 Slack 的 snapshot 发图片，除了消息相关 scope 之外，还要给 bot `files:write`，并且在变更 scope 后重新安装应用。
如果要让 Discord 在频道或私聊里识别前缀命令，需要在开发者后台启用 Message Content intent。
WhatsApp 首次运行会打印 QR 码，v1 仅支持私聊，不支持群组。默认情况下，命令必须来自另一个 WhatsApp 账号，`--allow-chat` 也应填写那个操作者账号的 JID。现在也支持实验性的 self-chat 模式：加上 `--whatsapp-allow-self-chat` 后，`--allow-chat` 应填写已配对账号自己的 JID，只接受来自另一个已关联设备发出的 self-chat 消息；为了避免回环，self-chat 模式会禁用裸文本，必须使用 `/send <text>`、`/enter <text>` 这类显式 slash 命令。

这里有两个特别容易踩坑的点：

- `--allow-chat` 必须和 daemon 实际收到的 WhatsApp chat ID 完全一致。self-chat 模式下，这个值经常是 `@lid`，而不是手机号形式的 `@s.whatsapp.net`。
- 如果这次启动没传 `--allow-chat`，以前 `config.toml` 里的 `allow_chats` 仍然会生效。实际优先级是：命令行参数 > 环境变量 > TOML 配置文件。

如果出现 `chat is not allowed to use this bot`，可以直接查 SQLite 里最近收到的 WhatsApp `chat_id`，原样填回 `--allow-chat`：

```bash
sqlite3 ~/.tmux-connect/tmux-connect.db \
  'select platform, chat_id, kind, body_preview, created_at from message_log order by id desc limit 10;'
```

常用参数：

- `--platform telegram|slack|discord|whatsapp`
- `--telegram-token TOKEN`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
- `--discord-token TOKEN`
- `--discord-command-prefix PREFIX`
- `--whatsapp-session-db PATH`
- `--whatsapp-device-name NAME`
- `--whatsapp-auto-mark-read`
- `--whatsapp-allow-self-chat`
- `--db PATH`
- `--allow-chat CHAT_ID`
- `--poll-timeout 20s`
- `--snapshot-lines 120`
- `--plain-text-mode type|execute`
- `--plain-text-echo off|snapshot`
- `--plain-text-echo-lines 12`
- `--plain-text-echo-delay 250ms`
- `--plain-text-echo-timeout 2s`
- `--telegram-snapshot-theme dark|light`
- `--telegram-snapshot-font-size 14`
- `--telegram-snapshot-font-file /path/to/font.ttf`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`
- `--telegram-api-base URL`

对应的 TOML 字段是 `[daemon].plain_text_mode`、
`[daemon].plain_text_echo`、`[daemon].plain_text_echo_lines`、
`[daemon].plain_text_echo_delay` 和 `[daemon].plain_text_echo_timeout`。

检查命令：

```bash
./tmux-connect daemon doctor --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
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

Discord 命令：

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

WhatsApp 命令：

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

Slack 频道里建议用 app mention 起命令，例如 `@tmux-connect panes`；Slack 私聊和 bot thread 里用 `tmux:` 作为命令前缀。带 `/` 的写法可能会先被 Slack 当成真正的 Slash Command 拦截，发不到 bot。Telegram、WhatsApp 以及 Slack 私聊和受管 thread 里的纯文本始终会指向当前 pane。默认仍是原始 `type` 模式，只输入不回车；如果设置 `--plain-text-mode execute` 或 `plain_text_mode = "execute"`，裸文本就会变成"发送并回车"。当 execute 模式配合 snapshot echo 使用时，daemon 会在 pane 输出发生可见变化后返回一段文本快照。若你在 execute 模式下仍想只输入不执行，使用 `/send <text>` 或 `tmux: send <text>`。发送 tmux 特殊键时，使用 `/keys` 或 `tmux: keys`，例如 `C-c`、`PageUp`、`F1`、`M-x`。`tmux: snapshot` 默认使用 `image`。Telegram snapshot 图片默认使用内置 `gomono` 字体、`14` pt 字号和 `dark` 主题。
Discord 里建议优先使用 Slash Commands。频道中也支持 `tmux: panes` 这类前缀命令；纯文本只会在 Discord 私聊里被当作 pane 输入，并沿用同样可配置的 `type`/`execute` 行为。
WhatsApp 里使用 `/panes`、`/follow on` 等斜杠命令。纯文本在受支持的私聊中指向当前 pane，沿用同样的 `type`/`execute` 行为。v1 仅支持私聊，群组消息会被忽略；当前实现同样会忽略已配对账号自己发出的 self-chat，因此需要用第二个 WhatsApp 账号给该已配对账号发消息。

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
- [docs/discord.md](./docs/discord.md) Discord 配置与操作
- [docs/whatsapp.md](./docs/whatsapp.md) WhatsApp 配置与操作
- [docs/discord-zh.md](./docs/discord-zh.md) Discord 接入指南（中文）
- [docs/architecture.md](./docs/architecture.md) 当前系统架构
- [docs/roadmap.md](./docs/roadmap.md) 近期路线图

## Current Limits

- 项目目前仍以 relay-first 为主，还没有结构化 agent 事件解析
- CLI 的专用控制键命令仅有 `enter` 和 `ctrl-c`；daemon 的 `/keys` 命令支持更多 tmux 按键名（`C-c`、`PageUp`、`F1`、`M-x` 等）
- follow 恢复状态尚不能跨 daemon 重启保留
- WhatsApp v1 仅支持私聊，不支持群组

## Acknowledgements

这个项目受到了 `cc-connect` 的启发，但方向不同：这里是围绕现有 pane 的 tmux-first relay，而不是围绕子进程生命周期的运行时。
