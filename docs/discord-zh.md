# Discord 接入指南

## 概述

tmux-connect 支持通过 Discord Bot 进行远程控制 tmux 会话。你可以通过 Discord 的斜杠命令（Slash Commands）或带前缀的消息（如 `tmux:`）来：

- 查看和管理 tmux 窗格（pane）
- 发送输入到指定的 tmux 窗格
- 获取终端快照（文本或图片）
- 实时跟踪终端输出（follow 模式）

## 功能特性

| 功能 | 支持情况 |
|------|----------|
| 斜杠命令（/panes, /select 等） | ✅ 完全支持 |
| 前缀命令（如 `tmux:`） | ✅ 支持 |
| 私聊直接输入 | ✅ 支持 |
| 图片快照 | ✅ 支持（通过 Embed） |
| 文本快照 | ✅ 支持 |
| 实时输出跟踪（follow） | ✅ 支持 |
| 线程支持 | ✅ 支持 |

---

## 第一步：创建 Discord Bot

### 1.1 创建 Discord Application

1. 访问 [Discord Developer Portal](https://discord.com/developers/applications)
2. 点击 **"New Application"** 按钮
3. 输入应用名称（建议如 `tmux-connect`）
4. 点击 **Create**

### 1.2 添加 Bot

1. 在左侧菜单选择 **Bot**
2. 点击 **Add Bot**
3. 点击 **Yes, do it!**

### 1.3 获取 Bot Token

1. 在 Bot 设置页面，找到 **Token** 部分
2. 点击 **Reset Token** 或复制现有 token
3. **重要**：立即将 token 保存到安全的地方，Discord 只会显示一次！

Token 格式类似：`MTAxMTExMTExMTExMTExMTE.MZAx.MMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM`

### 1.4 配置 Intents（意图）

Bot 需要特定的 intents 才能正常工作：

1. 在左侧菜单选择 **Bot**
2. 向下滚动到 **Privileged Gateway Intents** 部分
3. 启用以下 intents：

| Intent | 用途 |
|--------|------|
| **Message Content Intent** | 必须启用！用于接收前缀命令（`tmux:`）和私聊消息 |
| **Server Members Intent** | 可选，用于获取服务器成员信息 |
| **Presence Intent** | 可选，用于获取用户在线状态 |

### 1.5 配置权限

1. 在左侧菜单选择 **OAuth2** → **URL Generator**
2. 选择以下 scopes：
   - `bot`
   - `applications.commands`
3. 选择以下 permissions：
   - `View Channels` - 查看频道
   - `Send Messages` - 发送消息
   - `Attach Files` - 上传文件
   - `Create Public Threads` - 创建公开线程（如果需要）
   - `Send Messages in Threads` - 在线程中发送消息
4. 复制生成的 URL 并在浏览器中打开来邀请 Bot 到你的服务器

---

## 第二步：安装和配置 tmux-connect

### 2.1 环境要求

- Go 1.25 或更高版本
- tmux 已安装
- 守护进程使用内嵌 SQLite，不要求系统安装 sqlite3 命令行

### 2.2 构建项目

```bash
# 克隆项目
git clone https://github.com/hmgle/tmux-connect.git
cd tmux-connect

# 构建二进制文件
make build
```

如果你只打算使用 Discord，也可以裁掉不需要的平台：

```bash
make build PLATFORMS_INCLUDE=telegram,discord
```

### 2.3 配置方式

tmux-connect 支持三种配置方式，优先级：命令行参数 > 环境变量 > TOML 配置文件

#### 方式一：使用 TOML 配置文件（推荐）

创建配置文件 `~/.config/tmux-connect/config.toml`：

```toml
[daemon]
platform = "discord"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["discord:123456789012345678"]  # 可选白名单；删除这一行表示允许任意可访问的频道或私聊
snapshot_lines = 120
plain_text_mode = "execute"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"

[daemon.discord]
token = "your-discord-bot-token"
command_prefix = "tmux:"
```

这里的示例故意启用了 `plain_text_mode = "execute"`，这样在私聊里直接发 `continue` 之类的裸文本就会立刻执行。如果你更想保留“只输入、不回车”的行为，再改回 `type`。

`allow_chats` / `--allow-chat` 不是必填项。只有在你想限制“哪些 Discord 频道或私聊可以使用这个 Bot”时才需要配置；如果不配，任何能联系到这个 Bot 的频道或私聊都可以使用。

#### 方式二：使用环境变量

```bash
export TMUXCONN_PLATFORM=discord
export TMUXCONN_DISCORD_TOKEN="your-discord-bot-token"
export TMUXCONN_DISCORD_COMMAND_PREFIX="tmux:"
export TMUXCONN_DB_PATH="$HOME/.tmux-connect/tmux-connect.db"
export TMUXCONN_PLAIN_TEXT_MODE=execute
export TMUXCONN_PLAIN_TEXT_ECHO=snapshot
```

#### 方式三：使用命令行参数

```bash
./tmux-connect daemon run \
  --platform discord \
  --discord-token "your-discord-bot-token" \
  --discord-command-prefix "tmux:" \
  --plain-text-mode execute \
  --plain-text-echo snapshot \
  --db ~/.tmux-connect/tmux-connect.db
```

---

## 第三步：启动 Daemon

### 3.1 创建数据库目录

```bash
mkdir -p ~/.tmux-connect
```

### 3.2 健康检查

在启动之前，先验证配置是否正确：

```bash
./tmux-connect daemon doctor \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN"
```

预期输出：
```
./tmux-connect daemon doctor
discord token: ok
discord gateway intents: enable Message Content intent for prefix commands and DMs
sqlite store: ok (/path/to/tmux-connect.db)
tmux panes: ok (0 managed)
```

### 3.3 启动守护进程

```bash
# 前台运行
./tmux-connect daemon run

# 或者后台运行
nohup tmux-connect daemon run > /tmp/tmux-connect.log 2>&1 &

# 检查状态
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

预期输出：
```
./tmux-connect daemon status
db: /home/user/.tmux-connect/tmux-connect.db
registered chats: 0
bindings: 0
message log rows: 0
managed panes: 0
```

---

## 第四步：使用 Bot

### 4.1 私聊（Direct Messages）

在 Discord 中找到你的 Bot，直接发送消息即可。

**注意**：私聊中，纯文本消息始终会发送到当前选中的 tmux 窗格。默认 `type` 模式只输入不按 Enter；如果配置了 `plain_text_mode = "execute"` 或 `--plain-text-mode execute`，裸文本会直接“发送并回车”，并可返回一段文本快照回显。

### 4.2 服务器频道

在服务器频道中，需要使用命令来控制：

#### 斜杠命令（推荐）

```
/panes              # 列出所有可用的 tmux 窗格
/select <pane>      # 选择一个窗格，例如 /select %5
/current            # 显示当前选中的窗格
/snapshot [行数] [image|text]  # 获取终端快照
/send <text>        # 发送文本到当前窗格
/keys <按键>        # 发送特殊按键，如 /keys C-c, /keys PageUp
/enter [text]       # 发送文本并按 Enter
/ctrlc              # 发送 Ctrl-C
/clear              # 清除当前聊天与窗格的绑定
/unmanage <pane>    # 取消管理指定窗格
/follow on [间隔]|off  # 开启/关闭实时跟踪模式
```

#### 前缀命令（备选）

在频道中使用 `tmux:` 前缀：

```
tmux: panes
tmux: select %5
tmux: snapshot 120 image
tmux: send hello
tmux: keys C-c
tmux: enter continue
tmux: follow on 2s
tmux: follow off
```

### 4.3 典型使用流程

1. **启动 daemon 后，向 Bot 发送 `/panes`**

   Bot 会返回所有可用的 tmux 窗格列表

2. **选择一个窗格：`/select %5`**

   将该窗格绑定到当前 Discord 频道/私聊

3. **查看输出：`/snapshot`**

   获取终端的最后 120 行输出（默认）

4. **发送输入**

   - 在私聊中直接输入文本
   - 或使用 `/send <text>` 命令

5. **执行命令：使用 `/enter <text>`**

   发送文本并自动按 Enter

6. **实时跟踪：使用 `/follow on`**

   开启实时输出推送，可选指定间隔如 `/follow on 2s`

7. **停止跟踪：使用 `/follow off` 或 `/clear`**

---

## 命令详解

### /panes

列出所有当前 tmux 环境中可用的窗格。

**示例响应**：
```
Available panes:
• %0 - (bash) - 0:bash*
• %1 - (vim) - 1:vites*
• %5 - (python) - 2:python3
```

### /select <pane>

将指定窗格绑定到当前聊天。绑定后，该聊天的所有 tmux 相关命令都作用于这个窗格。

```bash
/select %5
```

### /snapshot [lines] [image|text]

获取终端快照。

| 参数 | 默认值 | 说明 |
|------|--------|------|
| lines | 120 | 快照行数 |
| mode | image | `image` 返回渲染图片，`text` 返回纯文本 |

```bash
/snapshot           # 获取 120 行图片快照
/snapshot 200       # 获取 200 行图片快照
/snapshot 100 text  # 获取 100 行文本快照
```

### /send <text>

显式发送文本到当前窗格。当你需要发送以 `/` 开头的文本时，这个命令很有用。

```bash
/send /exit
```

### /keys <key...>

发送 tmux 快捷键或控制键。

```bash
/keys C-c          # Ctrl-C
/keys C-d          # Ctrl-D (EOF)
/keys Escape       # Esc
/keys PageUp       # Page Up
/keys C-b C-c      # Ctrl-B 然后 Ctrl-C (tmux prefix)
/keys F1           # 功能键 F1
/keys M-x          # Alt+X
```

### /enter [text]

发送文本并自动按 Enter。如果不提供文本，则只发送 Enter。

```bash
/enter python3 main.py    # 发送文本并按 Enter
/enter                     # 只发送 Enter
```

### /ctrlc 或 /ctrl-c

发送 Ctrl-C 中断信号。

### /follow on [interval]|off

开启或关闭实时输出跟踪。

```bash
/follow on        # 使用默认间隔（700ms）
/follow on 1s     # 每秒推送一次
/follow on 2s     # 每两秒推送一次
/follow off       # 停止跟踪
```

### /current

显示当前绑定的 tmux 窗格信息。

### /clear

解除当前聊天与 tmux 窗格的绑定，并停止该聊天的 follow 模式。

### /unmanage <pane>

从 tmux-connect 管理中移除指定窗格，清理所有相关状态。

---

## 配置参考

### 完整配置示例

```toml
[tmux]
socket = "work"  # 可选：指定 tmux socket 名称

[daemon]
platform = "discord"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = [
    "discord:123456789012345678",      # 允许的频道
    "discord:987654321098765432",      # 允许的私聊
]
poll_timeout = "20s"
snapshot_lines = 120
follow_lines = 80
follow_min_interval = "700ms"
follow_debug = false

[daemon.discord]
token = "your-bot-token"
command_prefix = "tmux:"
```

### 命令行参数完整列表

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--platform` | 平台类型 | `telegram` |
| `--discord-token` | Discord Bot Token | - |
| `--discord-command-prefix` | 前缀命令前缀 | `tmux:` |
| `--db` | SQLite 数据库路径 | 必需 |
| `--allow-chat` | 可选白名单项，限制允许访问的频道/私聊 ID | - |
| `--snapshot-lines` | 快照默认行数 | 120 |
| `--follow-lines` | follow 默认行数 | 80 |
| `--follow-min-interval` | follow 最小推送间隔 | `700ms` |
| `--follow-debug` | 开启 follow 调试日志 | `false` |

### 环境变量完整列表

| 变量 | 说明 |
|------|------|
| `TMUXCONN_PLATFORM` | 平台类型 |
| `TMUXCONN_DISCORD_TOKEN` | Discord Bot Token |
| `TMUXCONN_DISCORD_COMMAND_PREFIX` | 命令前缀 |
| `TMUXCONN_DB_PATH` | 数据库路径 |
| `TMUXCONN_FOLLOW_DEBUG` | follow 调试模式 |

---

## 安全设置

### allow_chats 限制

使用 `--allow-chat` 参数限制只有特定频道/用户可以使用 Bot：

```bash
# 单个频道或私聊
--allow-chat discord:123456789012345678

# 多个频道/私聊
--allow-chat discord:123456789012345678 \
--allow-chat discord:987654321098765432
```

推荐格式是 `discord:<channel_id>` 或 `discord:<dm_id>`。不带 `discord:` 前缀的原始 ID 也能工作，但在多平台共用配置时更容易混淆。

### 如何获取 Discord 里的 ID

1. 打开 Discord 的 **用户设置** → **高级** → 打开 **开发者模式**
2. 对目标服务器频道或私聊会话点右键
3. 选择 **Copy Channel ID**

Discord 私聊的底层标识也是 channel ID，所以同样复制 **Channel ID** 即可。

如果你暂时还不确定具体值，也可以先不配置白名单，让 Bot 收到一条测试消息，再到 daemon 的 SQLite 数据库里查看最新一条 `platform = discord` 的 `message_log.chat_id`。

### 私聊安全

如果只想在私聊中使用，确保：
1. 不要把服务器频道 ID 放进 `allow_chats`
2. 只把你希望允许的私聊 ID 放进去

---

## 故障排除

### Bot 没有响应

1. **检查 Bot 是否在线**
   - 确认 Bot 已加入服务器且在线

2. **检查 Intents**
   - 访问 Discord Developer Portal
   - 确保 **Message Content Intent** 已启用
   - 重新邀请 Bot（有时候权限变更需要重新授权）

3. **检查 Token**
   ```bash
   tmux-connect daemon doctor --platform discord --discord-token "$TMUXCONN_DISCORD_TOKEN"
   ```

### 前缀命令不工作

1. 确认 `Message Content Intent` 已启用
2. 确认 Bot 有权限读取消息
3. 检查命令前缀是否正确（默认是 `tmux:`）

### 斜杠命令不显示

1. Discord 需要一些时间来注册全局斜杠命令
2. 尝试输入 `/` 看是否能显示命令列表
3. 重启 daemon 可能有助于刷新命令注册

### 快照图片发送失败

1. 确保 Bot 有 `Attach Files` 权限
2. 检查 Bot 是否在允许的频道中

### tmux 窗格列表为空

1. 确保目标 tmux pane 已经存在
2. 确认 tmux 正在运行
3. 尝试使用本地命令检查：
   ```bash
   tmux-connect list
   ```

### 数据库错误

1. 检查数据库目录是否有写入权限
2. 确认数据库文件没有被损坏或被其他工具异常占用
3. 尝试删除旧数据库重新创建：
   ```bash
   rm ~/.tmux-connect/tmux-connect.db
   tmux-connect daemon run
   ```

### 权限问题

确保 Bot 具有以下权限：
- `View Channels` (查看频道)
- `Send Messages` (发送消息)
- `Attach Files` (上传文件)
- `Create Public Threads` (创建公开线程，可选)
- `Send Messages in Threads` (在线程中发送消息，可选)

---

## 架构说明

tmux-connect 的 Discord 集成架构如下：

```
Discord Gateway
      │
      ▼
┌─────────────────────────────────────┐
│ internal/discord/client.go          │
│ - Discord Gateway 事件处理          │
│ - 斜杠命令注册                      │
│ - 消息/交互响应                     │
└─────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────┐
│ internal/daemon/discord_adapter.go  │
│ - 平台无关接口实现                  │
│ - 消息格式转换                      │
│ - 事件路由                          │
└─────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────┐
│ internal/daemon/router.go           │
│ - 命令解析和分发                    │
│ - 状态管理                          │
└─────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────┐
│ internal/tmuxconn/                   │
│ - tmux 操作封装                      │
└─────────────────────────────────────┘
```

### 关键文件

| 文件 | 作用 |
|------|------|
| `internal/discord/client.go` | Discord API 客户端 |
| `internal/daemon/discord_adapter.go` | 平台适配器 |
| `internal/daemon/router.go` | 命令路由器 |
| `internal/config/config.go` | 配置加载 |

---

## 常见问题

### Q: 如何控制多个 tmux 服务器？

为每个 tmux socket 启动独立的 daemon 实例，使用 `--socket` 参数：

```bash
./tmux-connect daemon run --socket work ...
./tmux-connect daemon run --socket home ...
```

### Q: 如何在服务器重启后自动启动 daemon？

使用 systemd 服务：

```ini
# /etc/systemd/system/tmux-connect.service
[Unit]
Description=tmux-connect Discord daemon
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/tmux-connect
ExecStart=/path/to/tmux-connect daemon run --platform discord --discord-token "YOUR_TOKEN" --db /home/username/.tmux-connect/tmux-connect.db
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

然后启用服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable tmux-connect
sudo systemctl start tmux-connect
```

### Q: 如何查看 daemon 日志？

如果使用 systemd：
```bash
journalctl -u tmux-connect -f
```

如果使用 nohup：
```bash
tail -f /tmp/tmux-connect.log
```

### Q: 为什么 follow 模式推送间隔不稳定？

这是为了避免 Discord 限流。可以使用 `--follow-min-interval` 调整：

```bash
--follow-min-interval 1s    # 最少 1 秒间隔
--follow-min-interval 500ms # 最小 500ms（可能触发限流）
```

### Q: 如何与其他 tmux-connect 平台共存？

每个 daemon 实例只能使用一个平台。要同时使用 Telegram 和 Discord，需要运行两个独立的 daemon 实例，使用不同的数据库路径。

---

## 参考链接

- [Discord Developer Portal](https://discord.com/developers/applications)
- [DiscordGo 库文档](https://pkg.go.dev/github.com/bwmarrin/discordgo)
- [tmux-connect 项目主页](https://github.com/hmgle/tmux-connect)
- [项目架构文档](./architecture.md)
