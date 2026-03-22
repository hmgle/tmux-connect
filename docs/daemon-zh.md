# Daemon 配置参考

中继 daemon 将 tmux pane 连接到聊天平台。与 CLI 共享相同的配置文件和优先级规则：命令行参数 > 环境变量 > TOML 配置。

## 配置文件

```toml
[tmux]
socket = "work"

[daemon]
platform = "telegram"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["123456789"]
poll_timeout = "20s"
snapshot_lines = 120
plain_text_mode = "type"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"
follow_debug = false

[daemon.telegram]
token = "123456:example-token"
snapshot_theme = "dark"
snapshot_font_size = 14
snapshot_font_file = "/path/to/font.ttf"

[daemon.slack]
bot_token = "xoxb-..."
app_token = "xapp-..."
command_prefix = "tmux:"

[daemon.discord]
token = "discord-bot-token"
command_prefix = "tmux:"

[daemon.whatsapp]
session_db = "/home/user/.tmux-connect/whatsapp-device.db"
device_name = "tmux-connect"
auto_mark_read = true
```

## 通用参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--platform` | 必填 | `telegram`、`slack`、`discord` 或 `whatsapp` |
| `--db PATH` | 必填 | SQLite 数据库路径 |
| `--allow-chat ID` | — | 限制特定 chat ID 的访问权限（可重复） |
| `--poll-timeout` | `20s` | 长轮询超时 |
| `--snapshot-lines` | `120` | 默认快照行数 |
| `--plain-text-mode` | `type` | `type`（原始输入）或 `execute`（输入 + 回车） |
| `--plain-text-echo` | `snapshot` | `off` 或 `snapshot`（execute 后回复 pane 输出） |
| `--plain-text-echo-lines` | `12` | echo 快照行数 |
| `--plain-text-echo-delay` | `250ms` | 抓取 echo 快照前的等待时间 |
| `--plain-text-echo-timeout` | `2s` | 等待可见输出变化的最大时间 |
| `--follow-lines` | `80` | 每次 follow 更新的最大行数 |
| `--follow-min-interval` | `700ms` | follow 更新之间的最小间隔（WhatsApp 未显式设置时默认 `2s`） |
| `--follow-debug` | `false` | 启用 follow 调试日志 |

### Telegram 专用

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--telegram-token` | — | Bot API token（或环境变量 `TMUXCONN_TELEGRAM_TOKEN`） |
| `--telegram-snapshot-theme` | `dark` | `dark` 或 `light` |
| `--telegram-snapshot-font-size` | `14` | 快照图片字号 |
| `--telegram-snapshot-font-file` | 内置 gomono | 自定义字体路径 |
| `--telegram-api-base` | — | 自定义 Telegram API 基础 URL |

### Slack 专用

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--slack-bot-token` | — | Bot token（`xoxb-...`） |
| `--slack-app-token` | — | App-level token（`xapp-...`），用于 Socket Mode |
| `--slack-command-prefix` | `tmux:` | 消息中的命令前缀 |

### Discord 专用

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--discord-token` | — | Bot token |
| `--discord-command-prefix` | `tmux:` | 文本命令前缀 |

### WhatsApp 专用

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--whatsapp-session-db` | — | 本地会话数据库路径（或环境变量 `TMUXCONN_WHATSAPP_SESSION_DB`） |
| `--whatsapp-device-name` | `tmux-connect` | 配对设备显示名称 |
| `--whatsapp-auto-mark-read` | `true` | 自动标记消息已读 |
| `--whatsapp-allow-self-chat` | `false` | 启用实验性 self-chat 模式 |

## 裸文本模式

默认情况下，裸文本以 `type` 模式发送到当前 pane——只输入不回车。设置 `--plain-text-mode execute` 后，裸文本会变成"发送并回车"。

当 execute 模式配合 `--plain-text-echo snapshot` 使用时，daemon 会在 pane 输出发生可见变化后返回一段文本快照。

若你在 execute 模式下仍想只输入不执行，使用 `/send <text>`。

## 诊断命令

```bash
# 验证 token、SQLite 存储和 tmux 访问
./tmux-connect daemon doctor --platform telegram --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"

# 显示 SQLite 记录数和受管 pane 数量
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

## 恢复模型

tmux 仍然是 pane 身份和管理元数据的事实来源。bridge 会通过 tmux user options 把恢复状态写回 pane：

- `@tmuxconn_managed=1`
- `@tmuxconn_mode=relay`
- `@tmuxconn_agent=<agent>`
- `@tmuxconn_label=<label>`
- `@tmuxconn_created_by=manual-attach`
- `@tmuxconn_last_activity_unix=<unix timestamp>`

remote daemon 会把平台聊天状态保存在 SQLite 中，包括绑定关系、当前 pane 选择、session 和消息关联。Schema 通过 `PRAGMA user_version` 进行版本管理。

## 平台指南

各平台的详细配置、命令列表和操作说明：

- [Telegram](./telegram.md)
- [Slack](./slack.md)
- [Discord](./discord.md)
- [WhatsApp](./whatsapp.md)
