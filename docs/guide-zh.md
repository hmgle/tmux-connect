# tmux-connect 中文快速开始

> 版本对应：当前主干（截至 2026-03-18）

## 这份文档适合谁

这是一份面向初次使用者的中文快速开始。

如果你只想尽快跑起来，先看这份文档。

如果你需要更细的说明，再看这些文档：

- [telegram.md](./telegram.md) — Telegram 专项配置和命令说明
- [slack.md](./slack.md) — Slack 专项配置和命令说明
- [discord.md](./discord.md) — Discord 专项配置和命令说明
- [discord-zh.md](./discord-zh.md) — Discord 接入详细指南（中文）
- [whatsapp.md](./whatsapp.md) — WhatsApp 专项配置和命令说明
- [architecture.md](./architecture.md) — 当前系统架构与恢复模型
- [troubleshooting-zh.md](./troubleshooting-zh.md) — 中文故障排查
- [roadmap.md](./roadmap.md) — 后续路线图

## 项目是什么

`tmux-connect` 是一个 tmux-first 的中继工具。

它不会创建或接管你的 AI Agent 进程，而是连接到已经存在的 tmux pane，让你通过：

- 本地 CLI 查看和控制 pane
- 本地 HTTP API 暴露同样的控制面
- Telegram、Slack、Discord 或 WhatsApp 远程查看输出、发送文本、回车、中断、开启 follow

## 前置条件

- Go `1.25` 或更新版本
- `tmux`
- 无需额外安装 `sqlite3` 命令行；daemon 使用内嵌 SQLite
- 一个已经存在的 tmux pane
- 一个 Telegram bot token、Slack bot/app token、Discord bot token 或已配对的 WhatsApp 设备（取决于你要用的平台）

## 五分钟上手

### 1. 构建或直接运行

```bash
make build
```

后文中的 `./tmux-connect` 也可以替换成 `go run ./cmd/tmux-connect`。

如果你不需要全部远程平台，也可以选择性构建：

```bash
# 只保留 Telegram 和 Slack
make build PLATFORMS_INCLUDE=telegram,slack

# 排除飞书和 WhatsApp
make build EXCLUDE=feishu,whatsapp
```

构建后可以运行 `./tmux-connect daemon help`，确认当前二进制包含哪些平台。

### 2. 在 tmux 里启动你的 Agent

```bash
tmux new -s dev
codex
```

也可以是 `claude`、`gemini` 或任何别的交互式终端程序。

### 3. 找到 pane 并纳入管理

```bash
./tmux-connect list
./tmux-connect attach --pane %5 --agent codex --label backend
./tmux-connect inspect --pane %5
```

这里的 `%5` 只是示例 pane ID。

### 4. 启动 Telegram daemon

```bash
export TMUXCONN_TELEGRAM_TOKEN="123456:ABC-DEF..."

./tmux-connect daemon run \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789
```

注意：

- 同一个 Telegram bot token 只支持一个活跃的 `tmux-connect daemon run` 实例
- 如果要管理多台机器，最简单的方式是每台机器使用不同的 bot

如果你还不知道 `chat_id`，见下文“获取 Telegram chat ID”。

### 5. 在 Telegram 里开始使用

按顺序发送：

```text
/panes
/select %5
/snapshot
continue
/enter
/follow on
```

如果需要中断：

```text
/keys C-c
```

关闭实时推送：

```text
/follow off
```

## 获取 Telegram chat ID

最简单的方法：

1. 在 Telegram 中搜索 `@userinfobot` 或 `@getmyid_bot`
2. 给它发消息
3. 读取你的 `chat_id`

也可以先给自己的 bot 发一条消息，然后在服务器上执行：

```bash
curl "https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates" | jq '.result[0].message.chat.id'
```

群组 `chat_id` 通常是负数。

## 常用命令

### Telegram 命令

| 命令 | 作用 |
|------|------|
| `/panes` | 列出 pane |
| `/select <pane>` | 选择当前聊天使用的 pane |
| `/current` | 查看当前 pane |
| `/snapshot [lines] [image\|text]` | 查看最近输出 |
| 直接发送文本 | 默认向当前 pane 发送原始文本；开启 execute 模式后会直接发送并回车 |
| `/send <text>` | 显式发送文本，适合文本本身以 `/` 开头 |
| `/keys <key...>` | 发送 tmux 组合键或功能键，如 `C-c`、`Enter`、`PageUp`、`F1`、`M-x` |
| `/enter [text]` | 发送回车；带文本时等于“发送文本并回车” |
| `/ctrlc` 或 `/ctrl-c` | 发送 Ctrl-C |
| `/follow on [interval]` | 开启实时推送 |
| `/follow off` | 关闭实时推送 |
| `/clear` | 清空当前 pane 选择 |
| `/unmanage <pane>` | 解除管理 |

说明：

- `/select` 会在 pane 尚未管理时自动 `attach`
- 直接发送的文本默认会进入当前 pane，但不会自动附带回车
- 如果设置 `--plain-text-mode execute`，直接发送文本就会变成“发送并回车”，并可配合文本快照回显
- 在默认 `type` 模式下，需要执行时，先发送文本，再发 `/enter`
- 也可以直接用 `/enter make test` 这样的一步写法
- `/keys` 用来发送 tmux key name，比如 `C-c`、`Enter`、`Escape`、方向键、`PageUp`、`F1-F12`、`C-a` 到 `C-z`、`M-x`
- 如果文本本身以 `/` 开头，使用 `/send <text>`
- `/snapshot` 默认发图片，显式写 `text` 才会发纯文本
- `/follow on` 默认聚合间隔是 `700ms`
- `/follow on 2s` 这样的写法也是支持的

### 本地 CLI

```bash
./tmux-connect list
./tmux-connect attach --pane %5 --agent codex --label backend
./tmux-connect detach --pane %5
./tmux-connect inspect --pane %5
./tmux-connect snapshot --pane %5 --lines 120
./tmux-connect send --pane %5 --text "continue" --enter
./tmux-connect enter --pane %5
./tmux-connect ctrl-c --pane %5
./tmux-connect stream --pane %5 --lines 120
./tmux-connect serve --listen 127.0.0.1:8080
./tmux-connect daemon run --telegram-token TOKEN --db ~/.tmux-connect/tmux-connect.db
./tmux-connect daemon doctor --telegram-token TOKEN
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

### HTTP API

启动：

```bash
./tmux-connect serve --listen 127.0.0.1:8080
```

常用端点：

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

示例：

```bash
curl http://127.0.0.1:8080/v1/panes

curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"continue","enter":true}'
```

## 常用 daemon 参数

```bash
./tmux-connect daemon run \
  --telegram-token "$TMUXCONN_TELEGRAM_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat 123456789 \
  --telegram-snapshot-theme dark \
  --telegram-snapshot-font-size 14 \
  --follow-lines 80 \
  --follow-min-interval 700ms
```

重要参数：

| 参数 | 说明 |
|------|------|
| `--telegram-token` | Telegram bot token |
| `--db` | SQLite 数据库路径 |
| `--allow-chat` | 允许访问的 chat ID，可重复传入 |
| `--poll-timeout` | 长轮询超时，默认 `20s` |
| `--snapshot-lines` | `/snapshot` 默认行数，默认 `120` |
| `--plain-text-mode` | 纯文本输入行为：`type` 或 `execute` |
| `--telegram-snapshot-theme` | `dark` 或 `light` |
| `--telegram-snapshot-font-size` | 图片字号，默认 `14` |
| `--telegram-snapshot-font-file` | 自定义字体文件 |
| `--follow-lines` | `/follow` 初始抓取行数，默认 `80` |
| `--follow-min-interval` | `/follow` 推送最小间隔，默认 `700ms` |
| `--follow-debug` | 输出 follow 调试日志 |
| `--telegram-api-base` | 自定义 Telegram Bot API 地址 |

如果开启 `--plain-text-mode execute`，还可以继续用
`--plain-text-echo off|snapshot`、`--plain-text-echo-lines`、
`--plain-text-echo-delay` 和 `--plain-text-echo-timeout`
控制执行后的文本快照回显行为。

运行约束：

- 同一个 bot token 上只应有一个活跃的 `tmux-connect daemon run`
- 如果要管理多台机器，建议每台机器使用不同的 bot

## 运行方式建议

- `tmux-connect daemon run` 建议放在单独的 tmux pane、`systemd`、`launchd` 或其他守护方式下运行
- 如果你只是本地调试，直接在终端里运行即可
- 如果你的 pane 很重要，建议先手动 `attach` 并加上 `--label`

## 当前恢复模型

恢复分成两层：

- tmux 保存 pane 级别的管理元数据
- SQLite 保存 Telegram 聊天绑定、当前 pane、sessions、message links

这意味着：

- daemon 重启后，聊天绑定和 reply continuity 可以恢复
- `follow` 订阅不会自动恢复，需要重新 `/follow on`

## 最常见的问题

- daemon 启不来：
  先跑 `tmux-connect daemon doctor --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"`
- Telegram 没反应：
  先检查 `--allow-chat`、bot token、网络连通性
- `/select` 后 pane 不可用：
  说明 tmux pane 已消失，重新 `tmux-connect list` 或 `/panes`
- `/follow on` 没输出：
  先用 `/snapshot` 看当前 pane 是否真的有新内容

更完整的排查见 [troubleshooting-zh.md](./troubleshooting-zh.md)。
