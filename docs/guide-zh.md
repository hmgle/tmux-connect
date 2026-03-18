# tmux-connect 使用指南（中文）

> 版本对应：Phase 3 foundation slice (2026-03-15)

## 目录

- [一、项目简介](#一项目简介)
- [二、系统架构](#二系统架构)
- [三、使用场景](#三使用场景)
- [四、前置条件](#四前置条件)
- [五、安装与构建](#五安装与构建)
- [六、操作步骤详解](#六操作步骤详解)
  - [6.1 本地 tmux 准备](#61-本地-tmux-准备)
  - [6.2 创建 Telegram Bot](#62-创建-telegram-bot)
  - [6.3 获取 Chat ID](#63-获取-chat-id)
  - [6.4 启动 Daemon](#64-启动-daemon)
  - [6.5 健康检查](#65-健康检查)
  - [6.6 Telegram 端操作](#66-telegram-端操作)
- [七、Telegram 命令参考](#七telegram-命令参考)
- [八、本地 CLI 命令参考](#八本地-cli-命令参考)
- [九、HTTP API 参考](#九http-api-参考)
- [十、工作流时序图](#十工作流时序图)
  - [10.1 基础操作时序](#101-基础操作时序)
  - [10.2 Follow 流式推送时序](#102-follow-流式推送时序)
  - [10.3 Daemon 重启恢复时序](#103-daemon-重启恢复时序)
- [十一、持久化与容灾](#十一持久化与容灾)
- [十二、端到端场景演示](#十二端到端场景演示)
- [十三、故障排查](#十三故障排查)
- [十四、项目源码结构](#十四项目源码结构)
- [十五、项目演进路线](#十五项目演进路线)

---

## 一、项目简介

**tmux-connect** 是一个 Go 编写的 tmux 桥接系统，让你通过 **Telegram**（手机/桌面客户端）远程操控服务器上 tmux 中运行的 AI 编程助手——包括 **Claude Code**、**Codex**、**Gemini CLI** 等。

核心设计理念：

- **tmux 是权威数据源**：bridge 不创建、不管理 pane 生命周期，只做中继
- **relay-first**：当前为纯文本中继模式，不解析 Agent 输出结构
- **持久化会话**：通过 SQLite 持久化聊天绑定和回复连续性，daemon 重启不丢失状态

适用人群：

- 需要远程监控和操控长时间运行的 AI Agent 的开发者
- 经常在移动端（手机/平板）处理开发任务的工程师
- 在多台服务器上同时运行多个 AI Agent 的团队

---

## 二、系统架构

### 整体架构图

```
┌──────────────────────────────────────────────────────────────┐
│                      Remote Server                           │
│                                                              │
│   ┌──────────┐   ┌──────────┐   ┌──────────┐               │
│   │ tmux %3  │   │ tmux %5  │   │ tmux %7  │  ← AI agents  │
│   │ Codex    │   │ Claude   │   │ Gemini   │    运行中      │
│   └────┬─────┘   └────┬─────┘   └────┬─────┘               │
│        │              │              │                       │
│        └──────────────┼──────────────┘                       │
│                       │                                      │
│               ┌───────▼────────┐                             │
│               │  tagb Service  │  ← tmux bridge 核心         │
│               │  (pane ops)    │    attach/detach/send/...   │
│               └───────┬────────┘                             │
│                       │                                      │
│          ┌────────────┼────────────┐                         │
│          │            │            │                          │
│   ┌──────▼──────┐ ┌───▼──────┐ ┌──▼───────┐                │
│   │  HTTP API   │ │  Daemon  │ │  SQLite  │                 │
│   │ (本地控制)  │ │  Router  │ │  Store   │                 │
│   │ :8080       │ │          │ │          │                 │
│   └─────────────┘ └────┬─────┘ └──────────┘                │
│                         │                                    │
│                  ┌──────▼──────┐                             │
│                  │  Telegram   │                             │
│                  │  Client     │                             │
│                  │ (long-poll) │                             │
│                  └──────┬──────┘                             │
└─────────────────────────┼────────────────────────────────────┘
                          │ HTTPS (Telegram Bot API)
                          │
                  ┌───────▼───────┐
                  │   Telegram    │
                  │   Cloud       │
                  └───────┬───────┘
                          │
              ┌───────────▼───────────┐
              │   Your Phone / PC     │
              │   Telegram Client     │
              └───────────────────────┘
```

### 模块职责

| 模块 | 目录 | 职责 |
|------|------|------|
| **tagb Service** | `internal/tagb/` | pane 控制操作（list/attach/detach/inspect/snapshot/send/stream），不感知 Telegram |
| **tmux Client** | `internal/tmux/` | tmux 执行层，pane 元数据读写，快照捕获，control-mode 流式输出 |
| **Telegram Client** | `internal/telegram/` | Telegram Bot API 封装：getUpdates 长轮询、sendMessage |
| **Daemon Router** | `internal/daemon/router.go` | 命令解析和 Telegram 处理器分发 |
| **Store** | `internal/daemon/store.go` | SQLite 持久层（聊天绑定、会话、消息链接） |
| **Follow Manager** | `internal/daemon/follow.go` | 输出流式推送，聚合和分块发送到 Telegram |
| **ReplyBus** | `internal/daemon/messenger.go` | 回复连续性逻辑，会话查找，消息链接 |
| **Registry** | `internal/daemon/registry.go` | 内存 pane 缓存，定期从 tmux 刷新 |
| **HTTP API** | `internal/httpapi/` | 本地 HTTP 控制面 |

### 数据流向

```
Telegram 消息 → getUpdates → Router 解析命令
                                  │
                    ┌─────────────┼─────────────┐
                    ▼             ▼             ▼
               Store 持久化   Service 操作   Registry 查询
                    │         (tmux 交互)       │
                    ▼             │             ▼
               SQLite DB         ▼          内存缓存
                              tmux pane
                                  │
                                  ▼
                            输出结果/快照
                                  │
                    ┌─────────────┤
                    ▼             ▼
              ReplyBus 构造   Follow Manager
              回复消息        聚合输出块
                    │             │
                    └──────┬──────┘
                           ▼
                    sendMessage → Telegram Cloud → Your Phone
```

---

## 三、使用场景

### 场景 1：通勤时继续审查 Claude Code 的输出

> 你在办公室启动了 Claude Code 处理一个大型重构任务，下班时它还在跑。
> 地铁上打开 Telegram → `/snapshot` 查看最新输出 → 发现它在等确认 → `/send y` + `/enter` 让它继续。

**关键价值**：不中断 AI Agent 的工作流，碎片时间也能推进开发。

### 场景 2：手机端监控多个 AI Agent 并行工作

> 服务器上 3 个 tmux pane 分别跑着 Claude Code、Codex、Gemini CLI 处理不同模块。
> Telegram 里 `/panes` 查看全部 → `/select %5` 切到 Claude Code → `/follow on` 实时推送 →
> 发现问题 `/ctrlc` 中断 → `/send "换个方案..."` + `/enter`。

**关键价值**：一个 Telegram 聊天窗口管理多个 Agent，随时切换。

### 场景 3：长时间部署任务的远程干预

> 远程服务器上 Claude Code 正在执行部署脚本，你需要在特定步骤输入确认。
> `/follow on` 开启实时流 → 看到提示后 `/send yes` + `/enter` → 部署继续。

**关键价值**：不必守在电脑前等待，手机收到推送后即时响应。

### 场景 4：团队协作观察

> 同一个 Telegram 群组（允许的 chat_id）里多个开发者可以 `/snapshot` 查看 Agent 进度，
> 由指定负责人执行 `/send` 操作，其他人只读观察。

**关键价值**：团队共享 AI Agent 工作进度的实时视图。

### 场景 5：Codex 审批流程远程操控

> Codex 在处理代码变更时需要人工审批才能继续。
> 你在外出时通过 Telegram `/snapshot` 看到审批请求 → `/send approve` + `/enter`。

**关键价值**：审批不受地点限制，AI Agent 不会因等待而空转。

---

## 四、前置条件

### 服务器端

| 依赖 | 要求 | 说明 |
|------|------|------|
| **Go** | >= 1.25.0 | 编译运行 tagb |
| **tmux** | 已安装 | pane 管理基础 |
| **sqlite3** | 已安装，在 PATH 中 | daemon 持久化存储 |
| **网络** | 能访问 `api.telegram.org` | Telegram Bot API |

### 客户端

| 依赖 | 要求 |
|------|------|
| **Telegram** | 任意平台的 Telegram 客户端（手机/桌面/Web） |

### AI Agent（按需）

在 tmux pane 中运行以下任一：

- **Claude Code**: `claude`
- **Codex**: `codex`
- **Gemini CLI**: `gemini`
- 或任何其他交互式终端程序

---

## 五、安装与构建

```bash
# 克隆仓库
git clone https://github.com/hmgle/tmux-connect.git
cd tmux-connect

# 验证 Go 版本
go version  # 需要 >= 1.25.0

# 编译（可选，也可以直接 go run）
go build -o tagb ./cmd/tagb

# 验证
./tagb --help
# 或
go run ./cmd/tagb --help
```

以下示例中，`tagb` 可替换为 `go run ./cmd/tagb`。

---

## 六、操作步骤详解

### 6.1 本地 tmux 准备

#### 步骤 1：启动 tmux 会话并运行 AI Agent

```bash
# 新建 tmux 会话
tmux new -s dev

# 在 pane 中启动 Claude Code（或 Codex / Gemini）
claude
```

如果你的 AI Agent 已经在某个 tmux pane 中运行，跳到步骤 2。

#### 步骤 2：查找 pane ID

在另一个终端（或 tmux 的另一个 pane）中：

```bash
# 列出所有 pane
tagb list

# 输出示例：
# Pane %5  session=dev  window=0  title=claude  (unmanaged)
# Pane %7  session=dev  window=1  title=bash    (unmanaged)
```

记下你要控制的 pane ID（如 `%5`）。

#### 步骤 3：将 pane 纳入 bridge 管理

```bash
# attach 并标记 agent 类型和标签
tagb attach --pane %5 --agent claude --label backend
```

支持的 `--agent` 类型：`unknown`、`claude`、`codex`、`gemini`、`cursor`

#### 步骤 4：验证管理状态

```bash
tagb inspect --pane %5

# 输出示例：
# Pane %5
#   managed: true
#   mode:    relay
#   agent:   claude
#   label:   backend
#   created: manual-attach
#   last_activity: 1710489600
```

此时 tmux pane 上已写入 bridge 元数据（`@tagb_managed=1` 等），即使 bridge 重启也不会丢失。

### 6.2 创建 Telegram Bot

1. 在 Telegram 中搜索 **@BotFather** 并打开对话
2. 发送 `/newbot`
3. 按提示输入 bot 名称和用户名
4. BotFather 返回一个 token，格式如 `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`
5. **保存好这个 token**

可选设置：
- `/setdescription` - 设置 bot 描述
- `/setcommands` - 设置命令菜单（方便 Telegram 中自动补全）：

```
panes - 列出所有 pane
select - 选择当前聊天操作的 pane
clear - 清空当前聊天选择的 pane
unmanage - 解除 pane 管理
current - 查看当前选择的 pane
snapshot - 截取 pane 输出
send - 向 pane 发送文本
enter - 发送回车
ctrlc - 发送 Ctrl-C
follow - 开启/关闭流式推送
```

### 6.3 获取 Chat ID

你需要知道 Telegram 聊天的 chat ID，用于 `--allow-chat` 白名单。

**方法 1：使用第三方 bot**

在 Telegram 中搜索 `@userinfobot` 或 `@getmyid_bot`，发送任意消息即可获取你的 chat ID。

**方法 2：通过 Bot API 获取**

先向你的 bot 发一条消息，然后：

```bash
curl "https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates" | jq '.result[0].message.chat.id'
```

**方法 3：群组 chat ID**

将 bot 加入群组后，在群组中发一条消息，然后用上述 curl 命令查看。群组 ID 通常为负数（如 `-1001234567890`）。

### 6.4 启动 Daemon

```bash
# 方式一：使用环境变量
export TAGB_TELEGRAM_TOKEN="123456:ABC-DEF..."
export TAGB_TELEGRAM_SNAPSHOT_THEME="light"
export TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE="16"
tagb daemon run \
  --db ~/.tagb/tagb.db \
  --allow-chat 987654321

# 方式二：使用命令行参数
tagb daemon run \
  --telegram-token "123456:ABC-DEF..." \
  --db ~/.tagb/tagb.db \
  --telegram-snapshot-theme light \
  --telegram-snapshot-font-size 16 \
  --telegram-snapshot-font-file /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf \
  --allow-chat 987654321
```

参数说明：

| 参数 | 说明 |
|------|------|
| `--telegram-token` | Telegram Bot token（也可用 `TAGB_TELEGRAM_TOKEN` 环境变量） |
| `--db` | SQLite 数据库路径（自动创建） |
| `--telegram-snapshot-theme` | 快照图片主题，支持 `dark`、`light`，默认 `dark` |
| `--telegram-snapshot-font-size` | 快照图片字号，默认 `14` |
| `--telegram-snapshot-font-file` | 快照图片字体文件路径，支持 `.ttf`、`.otf`；不设置时使用内置等宽字体 |
| `--allow-chat` | 允许的 chat ID（不设置则接受所有聊天） |

Daemon 启动后会：
1. 打开/创建 SQLite 数据库，执行 schema 迁移
2. 从 tmux 刷新 pane 清单
3. 排空 Telegram 离线期间积压的消息
4. 进入 `getUpdates` 长轮询循环

> 建议在 tmux 的另一个 pane 或使用 systemd/nohup 运行 daemon，确保它持续在后台运行。

### 6.5 健康检查

```bash
# 检查运行时依赖（tmux、sqlite3、Telegram 连通性）
tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"

# 查看持久化状态和管理的 pane 数量
tagb daemon status --db ~/.tagb/tagb.db
```

### 6.6 Telegram 端操作

现在打开 Telegram，找到你的 bot，开始操作：

```
第一步：查看可用 pane
你: /panes
Bot: Pane %5 [claude] "backend" (managed, selected)
     Pane %7 [unknown] (unmanaged)

第二步：选择 pane
你: /select %5
Bot: Selected %5

第三步：查看当前输出
你: /snapshot
Bot: [最近 120 行的 pane 输出]

第四步：向 Agent 发送指令
你: /send fix the authentication bug in login.go
Bot: Sent to %5
你: /enter
Bot: Enter sent

第五步：开启实时推送
你: /follow on
Bot: [初始快照]
Bot: [输出块 1...]
Bot: [输出块 2...]
...

第六步：中断 Agent
你: /ctrlc
Bot: Ctrl-C sent

第七步：关闭推送
你: /follow off
Bot: Follow stopped
```

---

## 七、Telegram 命令参考

### 管理类命令

| 命令 | 语法 | 说明 |
|------|------|------|
| `/panes` | `/panes` | 列出所有 pane，显示管理状态、选择信息、follow 状态 |
| `/select` | `/select <pane>` | 选择当前聊天操作的 pane；若该 pane 尚未管理，会自动纳入 bridge 管理 |
| `/clear` | `/clear` | 清空当前聊天选择的 pane，并停止该聊天的 follow |
| `/unmanage` | `/unmanage <pane>` | 解除 pane 管理，清除相关聊天绑定，停止关联的 follow |
| `/current` | `/current` | 显示当前聊天选择的 pane |

### 操作类命令

| 命令 | 语法 | 说明 |
|------|------|------|
| `/snapshot` | `/snapshot [lines] [image\|text]` | 截取当前 pane 最近 N 行输出（默认 120 行，默认 `image`；显式指定 `text` 时按文本消息发送） |
| `/send` | `/send <text>` | 向当前 pane 注入文本（不自动回车） |
| `/enter` | `/enter` | 发送回车键 |
| `/ctrlc` | `/ctrlc` 或 `/ctrl-c` | 发送 Ctrl-C 中断信号 |

### 流式推送命令

| 命令 | 语法 | 说明 |
|------|------|------|
| `/follow on` | `/follow on` | 开启实时输出推送（每 700ms 聚合一次） |
| `/follow off` | `/follow off` | 关闭实时输出推送 |

**注意事项：**

- `/select` 会在需要时自动管理 pane，无需先手动 `/attach`
- 每个聊天同一时间只能 follow 一个 pane
- 如果当前 pane 的 tmux session 消失，daemon 会自动清除当前选择并提示重新 `/select`

---

## 八、本地 CLI 命令参考

```bash
# 列出所有 pane
tagb list [--socket NAME] [--json]

# 管理 pane
tagb attach --pane %5 [--agent claude] [--label NAME]
tagb detach --pane %5

# 查看 pane 元数据
tagb inspect --pane %5

# 截取输出
tagb snapshot --pane %5 [--lines 120]

# 发送文本
tagb send --pane %5 --text "hello" [--enter]

# 发送控制键
tagb enter --pane %5
tagb ctrl-c --pane %5

# 流式输出（持续输出到终端）
tagb stream --pane %5 [--lines 120]

# 启动 HTTP API
tagb serve [--listen 127.0.0.1:8080]

# Daemon 操作
tagb daemon run --telegram-token TOKEN [--db PATH] [--allow-chat ID]
tagb daemon doctor --telegram-token TOKEN [--db PATH]
tagb daemon status [--db PATH]
```

全局选项：

| 选项 | 说明 |
|------|------|
| `--socket NAME` | tmux socket 名称（默认 "default"） |
| `--json` | JSON 格式输出 |

---

## 九、HTTP API 参考

启动 HTTP 服务：

```bash
tagb serve --listen 127.0.0.1:8080
```

### 端点列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 健康检查 |
| GET | `/v1/panes` | 列出所有 pane |
| POST | `/v1/panes/attach` | 管理 pane |
| POST | `/v1/panes/detach` | 解除管理 |
| GET | `/v1/panes/inspect?pane=%5` | 查看 pane 元数据 |
| GET | `/v1/panes/snapshot?pane=%5&lines=120` | 截取输出 |
| POST | `/v1/panes/send` | 发送文本 |
| POST | `/v1/panes/enter` | 发送回车 |
| POST | `/v1/panes/ctrl-c` | 发送 Ctrl-C |
| GET | `/v1/panes/stream?pane=%5&lines=120` | SSE 流式输出 |

### 示例

```bash
# 列出 pane
curl http://127.0.0.1:8080/v1/panes

# 发送文本并回车
curl -X POST http://127.0.0.1:8080/v1/panes/send \
  -H 'Content-Type: application/json' \
  -d '{"pane":"%5","text":"continue","enter":true}'

# 截取输出
curl "http://127.0.0.1:8080/v1/panes/snapshot?pane=%250&lines=50"
```

---

## 十、工作流时序图

### 10.1 基础操作时序

```
 You (Telegram)            Daemon (Router)           tmux (Claude Code)
      │                         │                          │
      │  /panes                 │                          │
      ├────────────────────────>│                          │
      │                         │── list-panes ───────────>│
      │                         │<── pane list ────────────│
      │  "3 panes found"        │                          │
      │<────────────────────────│                          │
      │                         │                          │
      │  /select %5             │                          │
      ├────────────────────────>│                          │
      │                         │── SQLite: save selection │
      │  "Selected %5"          │                          │
      │<────────────────────────│                          │
      │                         │                          │
      │  /snapshot              │                          │
      ├────────────────────────>│                          │
      │                         │── capture-pane ─────────>│
      │                         │<── 120 lines ────────────│
      │  "[pane output]"        │                          │
      │<────────────────────────│                          │
      │                         │                          │
      │  /send "fix the bug"    │                          │
      ├────────────────────────>│                          │
      │                         │── send-keys ────────────>│
      │  "Sent to %5"           │            "fix the bug" │
      │<────────────────────────│                          │
      │                         │                          │
      │  /enter                 │                          │
      ├────────────────────────>│                          │
      │                         │── send-keys Enter ──────>│
      │  "Enter sent"           │                          │
      │<────────────────────────│         [Agent 开始工作] │
      │                         │                          │
```

### 10.2 Follow 流式推送时序

```
 You (Telegram)            Daemon (FollowMgr)        tmux (Claude Code)
      │                         │                          │
      │  /follow on             │                          │
      ├────────────────────────>│                          │
      │                         │── subscribe ────────────>│
      │                         │   (control-mode /        │
      │                         │    polling fallback)     │
      │                         │<── initial snapshot ─────│
      │  "[初始快照]"            │                          │
      │<────────────────────────│                          │
      │                         │                          │
      │                         │   [Agent 输出代码变更...] │
      │                         │<── output chunk 1 ───────│
      │                         │   (700ms 聚合窗口)       │
      │  "chunk 1"              │                          │
      │<────────────────────────│                          │
      │                         │                          │
      │                         │<── output chunk 2 ───────│
      │  "chunk 2"              │                          │
      │<────────────────────────│                          │
      │        ...              │         ...              │
      │                         │                          │
      │  /follow off            │                          │
      ├────────────────────────>│                          │
      │                         │── unsubscribe ──────────>│
      │  "Follow stopped"       │                          │
      │<────────────────────────│                          │
```

**Follow 机制说明：**

- 输出通过 tmux control-mode 实时订阅（不可用时降级为轮询快照）
- 每 700ms 聚合一次输出，避免 Telegram 消息频率限制
- 每条 Telegram 消息最大约 3500 字符，超长自动分块
- 所有 follow 消息会 reply 到触发 `/follow on` 的消息，形成 Telegram 对话线索
- 每个聊天同时只能有一个活跃的 follow 订阅

### 10.3 Daemon 重启恢复时序

```
                            Daemon 崩溃/重启
                                  │
                                  ▼
                        ┌─────────────────┐
                        │  启动初始化      │
                        └────────┬────────┘
                                 │
                    ┌────────────┼────────────┐
                    ▼            ▼            ▼
             从 tmux 刷新    从 SQLite      排空 Telegram
             pane 清单       恢复状态       积压消息
                    │            │            │
                    │     ┌──────┼──────┐     │
                    │     ▼      ▼      ▼     │
                    │  bindings state sessions │
                    │     │      │      │     │
                    └─────┼──────┼──────┼─────┘
                          ▼      ▼      ▼
                    ┌─────────────────────────┐
                    │  恢复完成，进入长轮询    │
                    └─────────────────────────┘

恢复内容：
  [自动恢复] chat 绑定关系 (chat_bindings)
  [自动恢复] 当前操作 pane (chat_state)
  [自动恢复] 回复线程锚点 (sessions)
  [需手动]   follow 订阅 → 需要用户重新 /follow on
```

### 10.4 完整生命周期时序

```
┌────────────────────────────────────────────────────────────────────┐
│                        完整使用流程                                │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  1. 准备阶段（服务器端，一次性操作）                               │
│     tmux new -s dev                                                │
│     claude                        ← 启动 AI Agent                 │
│     tagb attach --pane %5         ← 标记 pane                     │
│     tagb daemon run ...           ← 启动 daemon                   │
│                                                                    │
│  2. 连接阶段（Telegram 端）                                        │
│     /panes                        ← 发现可用 pane                  │
│     /select %5                    ← 选择目标 pane                 │
│                                                                    │
│  3. 工作阶段（反复循环）                                           │
│     /snapshot                     ← 查看当前状态                   │
│     /send <指令>                  ← 发送命令                       │
│     /enter                        ← 确认执行                       │
│     /follow on                    ← 实时观察                       │
│     ... 等待输出 ...                                                │
│     /ctrlc                        ← 需要时中断                     │
│     /follow off                   ← 暂停推送                       │
│                                                                    │
│  4. 切换阶段（需要操作其他 Agent 时）                              │
│     /panes                        ← 查看全部                       │
│     /select %7                    ← 切换到另一个 pane              │
│     /snapshot                     ← 继续工作                       │
│                                                                    │
│  5. 结束（可选）                                                   │
│     /unmanage %5                  ← 解除管理                       │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## 十一、持久化与容灾

### SQLite 数据库结构

```
              SQLite Store (~/.tagb/tagb.db)
    ┌────────────────────────────────────────────┐
    │                                            │
    │  Phase 2 表:                               │
    │  ├── chat_bindings                         │
    │  │   (chat_id, pane_key)                   │
    │  │   → 哪个 chat 绑定了哪个 pane           │
    │  │                                         │
    │  ├── chat_state                            │
    │  │   (chat_id, current_pane_key)           │
    │  │   → 每个 chat 当前操作的 pane            │
    │  │                                         │
    │  └── message_log                           │
    │      (chat_id, pane_key, direction,        │
    │       kind, telegram_message_id, body)     │
    │      → 审计日志                             │
    │                                            │
    │  Phase 3 表:                               │
    │  ├── sessions                              │
    │  │   (session_key, platform, chat_id,      │
    │  │    pane_key, agent, last_inbound_id,    │
    │  │    last_outbound_id, updated_at)        │
    │  │   → 回复连续性会话                       │
    │  │                                         │
    │  └── message_links                         │
    │      (platform, chat_id, pane_key,         │
    │       session_key, kind, inbound_id,       │
    │       outbound_id, reply_to_id)            │
    │      → Telegram 消息 ID 追踪               │
    │                                            │
    └────────────────────────────────────────────┘
```

### 双层恢复模型

| 存储层 | 内容 | 恢复方式 |
|--------|------|----------|
| **tmux 元数据** | managed 状态、agent 类型、label、模式 | 自动（tmux 原生持久化） |
| **SQLite** | 聊天绑定、当前 pane、回复锚点、审计日志 | 自动（daemon 启动时读取） |
| **内存** | follow 订阅、pane 缓存 | 需手动恢复（重新 `/follow on`） |

### 回复连续性（Phase 3）

- 每条 pane 相关命令都会创建/更新一个 session：`telegram:<chat_id>:<pane_key>`
- 出站消息自动 reply 到该 session 最近的入站消息
- 在 Telegram 中形成清晰的对话线索
- daemon 重启后，reply chain 从 SQLite 恢复，不中断

### Schema 版本管理

通过 `PRAGMA user_version` 追踪 schema 版本，支持增量迁移：

- Version 0 → 1：Phase 2 基础表（chat_bindings, chat_state, message_log）
- Version 1 → 2：Phase 3 新增表（sessions, message_links）

---

## 十二、端到端场景演示

### 演示：远程让 Claude Code 修复 Bug

```
前提：Claude Code 已在服务器 tmux 中运行，daemon 已启动

===== 手机 Telegram 操作 =====

1. 查看可用 pane
   你: /panes
   Bot: %5 claude "backend" [managed] [selected]
        %7 gemini "frontend" [managed]

2. 确认当前选择
   你: /current
   Bot: Current pane: %5 (claude, backend)

3. 查看 Claude Code 当前状态
   你: /snapshot 50
   Bot: ... (最近 50 行输出)
        > Waiting for input...

4. 发送修复指令
   你: /send fix the authentication bug in login.go, make sure to add proper error handling
   Bot: Sent to %5
   你: /enter
   Bot: Enter sent

5. 开启实时推送，观察工作过程
   你: /follow on
   Bot: [初始快照]
   Bot: Reading login.go...
   Bot: Found the issue: missing nil check on line 42...
   Bot: Editing login.go...
   Bot: ... (持续推送)

6. Claude Code 询问是否运行测试
   Bot: Should I run the tests? (y/n)
   你: /send y
   Bot: Sent to %5
   你: /enter
   Bot: Enter sent

7. 观察测试结果
   Bot: Running tests...
   Bot: === RUN TestLogin
   Bot: === RUN TestLoginWithInvalidCredentials
   Bot: PASS (2 tests, 0 failures)

8. 任务完成，关闭推送
   你: /follow off
   Bot: Follow stopped

整个过程你可能在地铁上、咖啡厅里，
只需要一部能上 Telegram 的手机。
```

### 演示：多 Agent 切换管理

```
===== 同时管理 3 个 AI Agent =====

1. 查看全部
   你: /panes
   Bot: %3 codex "api-refactor" [managed]
        %5 claude "backend-fix" [managed] [selected] [following]
        %7 gemini "docs-gen" [managed]

2. Claude Code 任务完成，切换到 Codex
   你: /follow off
   Bot: Follow stopped
   你: /select %3
   Bot: Selected %3

3. 查看 Codex 进度
   你: /snapshot
   Bot: ... Codex 输出 ...
        [Waiting for approval]

4. 审批
   你: /send approve
   Bot: Sent to %3
   你: /enter
   Bot: Enter sent

5. 切换到 Gemini 检查文档生成
   你: /select %7
   Bot: Selected %7
   你: /snapshot
   Bot: ... Gemini 文档输出 ...
```

---

## 十三、故障排查

### 常见问题

#### Daemon 启动失败

```bash
# 检查依赖
tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"

# 常见原因：
# - sqlite3 未安装或不在 PATH 中
# - Telegram token 无效或过期
# - 无法访问 api.telegram.org（网络/代理问题）
```

#### Telegram 命令无响应

| 可能原因 | 解决方法 |
|----------|----------|
| chat ID 不在允许列表 | 检查 `--allow-chat` 参数 |
| Daemon 已崩溃 | 检查 daemon 进程是否存活 |
| 网络问题 | 确认服务器能访问 Telegram API |
| Bot token 错误 | 用 `daemon doctor` 验证 |

#### `/select` 首次选择未管理 pane

```bash
# 直接在 Telegram 中选择即可，daemon 会自动 attach
/select %5
```

#### `/follow on` 后没有输出

- Agent 可能处于空闲状态（没有新输出）
- 尝试 `/snapshot` 确认 pane 当前内容
- 检查 Agent 是否在等待输入

#### Daemon 重启后 follow 失效

这是设计行为：follow 订阅是内存状态，不会跨 daemon 重启持久化。

```
# 重启后只需重新开启
/follow on
```

#### pane 消失后的行为

当 tmux pane 被关闭或 tmux 会话结束：
- Daemon 检测到 pane 不存在
- 自动清除相关聊天的 current_pane 状态
- 提示用户重新 `/select`

### 退出码参考

| 退出码 | 含义 |
|--------|------|
| 0 | 成功 |
| 2 | 输入参数无效 |
| 3 | pane 未找到或 pane 操作失败 |
| 4 | tmux 通信失败 |

---

## 十四、项目源码结构

```
tmux-connect-cx/
├── cmd/tagb/
│   └── main.go                    # CLI 入口
│
├── internal/
│   ├── tagb/
│   │   ├── app.go                 # CLI 应用定义
│   │   ├── service.go             # 核心 bridge service
│   │   └── errors.go              # 错误类型和退出码
│   │
│   ├── tmux/
│   │   ├── client.go              # tmux 执行和流式输出
│   │   ├── metadata.go            # bridge 元数据类型
│   │   ├── types.go               # Target, PaneInfo, OutputChunk
│   │   ├── control.go             # control-mode 流式
│   │   ├── polling.go             # 轮询降级方案
│   │   └── target.go              # pane 目标解析
│   │
│   ├── telegram/
│   │   └── client.go              # Telegram Bot API 封装
│   │
│   ├── daemon/
│   │   ├── cli.go                 # Daemon 命令分发 (run/doctor/status)
│   │   ├── router.go              # Telegram 命令路由
│   │   ├── store.go               # SQLite 持久层
│   │   ├── messenger.go           # ReplyBus 回复连续性
│   │   ├── follow.go              # Follow 管理器
│   │   └── registry.go            # Pane 注册表（内存缓存）
│   │
│   └── httpapi/
│       └── server.go              # HTTP API 服务
│
├── docs/
│   ├── README.md                  # 文档索引
│   ├── guide-zh.md                # 本文件（中文使用指南）
│   ├── telegram.md                # Telegram 设置指南
│   ├── architecture-phase1.md     # Phase 1 架构
│   ├── architecture-phase2.md     # Phase 2 架构
│   ├── design-tmux-bridge.md      # 设计文档
│   ├── phase3-status.md           # Phase 3 状态
│   ├── phase3-handoff.md          # Phase 3 交接文档
│   ├── phase4-handoff.md          # Phase 4 方向
│   └── ...                        # 其他规格文档
│
├── go.mod
├── README.md
└── CLAUDE.md
```

---

## 十五、项目演进路线

| Phase | 状态 | 内容 | 关键特性 |
|-------|------|------|----------|
| **Phase 1** | 完成 | 本地 tmux bridge | attach/detach/snapshot/send/stream、HTTP API |
| **Phase 2** | 完成 | Telegram relay daemon | 长轮询、命令路由、SQLite 持久化、follow 推送 |
| **Phase 3** | 完成 | 持久回复连续性 | sessions + message_links、schema 迁移、reply chain 恢复 |
| **Phase 4** | 计划中 | 结构化 Agent 输出解析 | 适配器架构、识别审批/代码块/测试结果 |
| **未来** | 规划中 | Telegram inline UI | callback buttons、一键审批/继续/中断 |
| **未来** | 规划中 | 多平台连接器 | 支持 Slack / Discord 等其他消息平台 |

### Phase 4 重点方向

- **结构化适配器**：按 agent 类型（Claude/Codex/Gemini）解析输出，识别高级事件
- **降级到 relay**：适配器失败时自动回退纯文本中继，保证可靠性
- **Codex 优先**：以 Codex 审批流为首个结构化目标
- **Telegram 内联操作**：基于 message_links 支持 callback buttons（审批、继续、停止）
- **不破坏现有功能**：保持 tmux-first 模型、relay 模式、纯文本降级

---

## 附录：tmux 快速参考

如果你不熟悉 tmux，这里是常用操作：

```bash
# 新建会话
tmux new -s <session-name>

# 列出会话
tmux ls

# 进入已有会话
tmux attach -t <session-name>

# 分离（不关闭）
Ctrl-b d

# 在会话内新建 pane
Ctrl-b %    # 水平分割
Ctrl-b "    # 垂直分割

# 列出所有 pane（含 ID）
tmux list-panes -a -F "#{pane_id} #{session_name}:#{window_index}"

# 查看特定 pane 输出
tmux capture-pane -p -t %5 -S -120
```
