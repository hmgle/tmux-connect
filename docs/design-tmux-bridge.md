# tmux Agent Mobile Bridge — 初步设计

> 状态：设计阶段 | 日期：2026-03-13
>
> 本文档描述一个独立的新项目：将运行在 tmux 中的 Claude Code、Codex CLI、Gemini CLI、
> Cursor Agent 等工具桥接到 Telegram、飞书等移动端消息平台，支持远程查看、发送指令、
> 批准权限、切换会话与恢复连接。

## 1. 设计目标

这个项目的核心需求不是“替代 cc-connect 的 pipe 通信”，而是：

1. 用户在桌面或服务器上通过 tmux 运行 AI coding agent
2. 用户离开电脑后，仍能在手机上的 Telegram、飞书等 APP 中：
   - 查看 agent 当前输出和最近屏幕内容
   - 向 agent 继续发送 prompt、命令或中断信号
   - 处理权限确认、继续执行、停止执行
   - 在多个 tmux session/window/pane 之间切换
3. Bridge 进程重启后，尽量能恢复对已有 tmux pane 的控制

因此，这个项目的“事实来源”应是 tmux，而不是 bridge 自己持有的子进程句柄。

## 2. 非目标

第一版不追求以下目标：

- 不追求兼容 cc-connect 的 `core.AgentSession` 接口
- 不要求把所有 agent 都抽象成统一的“长驻 stdin JSON 会话”
- 不要求接管任意 shell 里手工启动、且没有任何元数据标记的历史进程
- 不追求在移动端完整复刻桌面终端体验
- 不优先支持 Windows

## 3. 使用场景拆分

从真实需求出发，这个项目其实有两种模式，文档原版只覆盖了其中一半。

### 3.1 模式 A：结构化 Agent Bridge

适用于 Claude Code、Codex CLI、Gemini CLI、Cursor Agent 这类支持 JSON/stream-json
输出的 agent。

能力：

- 将 agent 输出解析为结构化事件
- 在移动端更自然地展示“思考 / 工具调用 / 最终回复”
- 对支持权限交互的 agent 发送“允许 / 拒绝”
- 知道 agent 侧 session/thread/chat ID，支持恢复会话

### 3.2 模式 B：通用 Terminal Relay

适用于任意 tmux pane，即使里面跑的不是 AI agent，或者 agent 没有稳定的 JSON 协议。

能力：

- 读取 pane 的实时输出和屏幕快照
- 向 pane 发送文本、Enter、Ctrl-C 等控制输入
- 从移动端完成“查看”和“基本控制”

限制：

- 无法可靠识别结构化事件
- 无法提供高质量权限卡片或会话元数据
- 更接近“远程终端代理”，不是“agent 协议桥”

这个能力分层很重要。它决定了项目不应只设计成“AgentAdapter + JSON parser”，而应设计成：

- 底层：tmux terminal relay
- 上层：按 agent 类型挂可选的 structured adapter

## 4. 产品形态

推荐把项目定义为一个独立守护进程，例如：

```text
tmux-agent-bridge
```

它负责：

- 连接 tmux control mode
- 发现和管理 tmux target
- 连接消息平台机器人
- 在“移动端会话”和“tmux pane”之间做路由
- 对部分 agent 做结构化增强

高层架构：

```text
Mobile Apps (Telegram / Feishu / ...)
                |
                v
      Platform Connectors
                |
                v
         Bridge Daemon
    +----------------------+
    | Session Router       |
    | Pane Registry        |
    | Permission Manager   |
    | Screen Cache         |
    | Agent Profiles       |
    +----------------------+
                |
                v
       tmux Control Client
                |
                v
            tmux server
                |
                v
         panes/windows/session
```

## 5. 核心设计调整

相对于原文档，下面这些点需要调整。

### 5.1 不再把“替代 pipe”作为主叙事

原文档把 tmux 方案描述成 cc-connect pipe 模式的替代品。这不符合新项目目标。

对独立项目来说，tmux 不是“底层传输层之一”，而是整个系统的核心运行时：

- 用户可直接 attach 观察
- bridge 可通过 control mode 读写
- bridge 崩溃后 tmux pane 还在
- 用户甚至可以脱离 bridge 手动在 tmux 内操作

### 5.2 Session 的主键应是 tmux target，不是平台会话

原文档更像是“平台 session_key 映射到 agent session”。

独立项目下更合理的主键关系是：

```text
mobile_chat_id -> bridge_conversation
bridge_conversation -> tmux_target
tmux_target -> optional agent metadata
```

其中 `tmux_target` 可以是：

- `session:window.pane`
- tmux `pane_id`，如 `%5`
- 或者带 socket 名的完整标识，如 `main:%5`

移动端对话只是控制入口，不应成为底层 session 的唯一身份。

### 5.3 需要“托管模式”与“附着模式”

原文档只详细描述了 bridge 创建 pane 的场景。真实需求下至少有两种来源：

1. 托管模式：bridge 创建新 window/pane，并在里面启动 agent
2. 附着模式：用户把一个现有 pane 注册给 bridge 进行远程控制

附着模式才真正匹配“tmux 里已经在跑 agent，我想用手机接管/查看”的需求。

为此需要一个显式注册动作，例如：

```bash
tagb attach-pane --pane %5 --agent claude --chat telegram:123456
```

或者在 tmux 内执行：

```bash
tagb register --agent claude --pane %5
```

注册后 bridge 应写入元数据，而不是只靠窗口标题猜。

### 5.4 元数据必须写入 tmux user options

不要把 `window_name = session_key` 当成恢复依据。标题只是展示层，用户会改名。

建议使用 tmux user options 保存桥接元数据：

```text
@tagb_managed=1
@tagb_agent=claude
@tagb_mode=structured
@tagb_chat_binding=telegram:123456
@tagb_session_id=...
@tagb_thread_id=...
@tagb_last_activity_unix=...
```

重启恢复时，bridge 应：

1. 枚举 tmux pane
2. 读取 `@tagb_*` 元数据
3. 恢复 pane registry
4. 对已注册 pane 重建输出订阅

## 6. 新的核心组件

### 6.1 TmuxClient

职责：

- 建立单一 tmux control mode 连接
- 执行 tmux 命令
- 分发 `%output`、`%exit`、`%subscription-changed` 等异步通知
- 提供 pane snapshot、输入注入、signal 发送、metadata 读写 API

建议能力：

- `Exec(args ...string) (string, error)`
- `SubscribePaneOutput(paneID string) <-chan []byte`
- `InjectInput(paneID string, data []byte) error`
- `SendKeys(paneID string, keys ...string) error`
- `SendSignal(paneID string, signal string) error`
- `CapturePane(paneID string, history int) (string, error)`
- `GetUserOption(target, key string) (string, error)`
- `SetUserOption(target, key, value string) error`

注：

- 长文本输入优先走 tmux 的 stdin 注入能力，不要依赖 shell 拼参
- `Ctrl-C`、`Enter`、方向键这类控制输入仍需要 `send-keys`

### 6.2 PaneRegistry

职责：

- 保存 bridge 已知 pane 的元数据
- 维护 pane 与移动端会话的绑定
- 记录 agent 类型、结构化模式、最近活动时间

它的状态来源分两层：

1. tmux 内 user options
2. 本地持久化数据库

建议：

- tmux 内存储“恢复所必需”的最小元数据
- 本地 SQLite/BoltDB 存储更丰富的索引、消息映射、权限状态、审计日志

### 6.3 ScreenService

这是原文档里缺失但对移动端非常关键的组件。

职责：

- 把 pane 的实时输出转成适合手机阅读的消息流
- 提供“最近屏幕快照”
- 做节流、聚合和去噪

因为移动端不是终端，不能把每个 `%output` chunk 都直接发出去。需要：

- 行缓冲
- 时间窗口聚合，例如 300ms 到 1s 合并一次
- 对重复 prompt/shell 回显做裁剪
- 对长屏输出支持“发送摘要 + 点击查看全文/快照”

### 6.4 CommandRouter

职责：

- 将移动端指令路由到对应 pane
- 区分“聊天消息”和“终端控制命令”

建议移动端至少支持：

- `/bind`
- `/list`
- `/switch`
- `/snapshot`
- `/send <text>`
- `/enter`
- `/ctrl-c`
- `/stop`
- `/resume`
- `/detach`

其中：

- `/send` 发文本但不自动回车，可用于补全输入
- 普通消息默认等价于“发送文本并回车”

### 6.5 AgentProfile

原文档中的 `AgentAdapter` 需要保留，但职责应收缩。

对独立项目来说，AgentProfile 只负责“结构化增强”，不负责承载整个系统。

```go
type AgentProfile interface {
    Name() string
    Detect(paneMeta PaneMeta, snapshot string) bool
    ParseOutput(line []byte) ([]BridgeEvent, error)
    EncodeUserInput(text string, files []FileRef) ([]byte, error)
    EncodePermissionResponse(req PermissionRequest, allow bool) ([]byte, error)
    SupportsStructuredInput() bool
    SupportsPermissions() bool
}
```

没有匹配到 profile 时，自动降级到 Terminal Relay 模式。

## 7. 数据流

### 7.1 托管创建

```text
手机发送 /new claude
  -> Bridge 创建 tmux window
  -> 写入 @tagb_* 元数据
  -> 启动 claude --output-format stream-json ...
  -> 绑定当前 mobile conversation <-> pane
  -> 开始订阅输出
```

### 7.2 附着已有 pane

```text
用户在桌面 tmux 中已有 %5 正在运行 codex/claude
  -> 用户执行 tagb attach-pane --pane %5 --agent auto
  -> Bridge 读取 snapshot 做 agent detection
  -> 写入 @tagb_* 元数据
  -> 绑定到指定 mobile conversation
  -> 后续从手机继续查看/发送
```

### 7.3 输出同步

```text
tmux %output
  -> TmuxClient unescape
  -> ScreenService 聚合
  -> 如果存在 AgentProfile:
       解析为 thinking/tool/text/result/permission
     否则:
       作为普通终端文本输出
  -> 平台 connector 发到 Telegram/飞书
```

### 7.4 输入同步

分两类：

1. 终端控制输入
   - `Enter`
   - `Ctrl-C`
   - `Esc`
   - `Up`
   - `Down`

2. 结构化 agent 输入
   - Claude Code 的 `stdin stream-json`
   - 权限响应 JSON
   - 未来可能的文件/图片消息编码

原则：

- 能结构化输入的，优先走结构化
- 不能结构化输入的，退化为文本注入或命令注入

## 8. 关于不同 agent 的现实约束

这一节是对原文档最需要修正的地方。

### 8.1 Claude Code

最适合第一阶段支持。

原因：

- 长驻进程模型适合 tmux pane
- `stream-json` 输出稳定
- stdin 可持续注入
- 权限请求/响应机制明确

因此 Claude Code 应作为第一优先级的 structured profile。

### 8.2 Codex CLI / Gemini CLI / Cursor Agent / OpenCode

这些工具在当前生态里大多更像“逐轮启动一个进程，再靠 session ID 恢复上下文”。

这意味着：

- 它们适合“托管模式”
- 不太适合“附着到一个已经退出、只剩 shell prompt 的 pane 后继续做结构化多轮”

因此建议把它们的支持拆成两级：

1. 基础级
   - 能在托管模式中启动
   - 能读取 JSON 输出
   - 能在移动端展示结果

2. 增强级
   - 能在 tmux 中保留一个专用 pane
   - 每次新消息触发一次新的 agent 子进程
   - bridge 负责把 thread/session ID 写入 pane metadata

不要把这类工具建模成“等待 shell prompt，再 send-keys 新命令”作为核心路径。
更稳的方式是：

- 保留该 pane 为 bridge 专用 pane
- 每轮使用 `respawn-pane` 或直接在新 pane 中执行 agent 命令
- 完成后 pane 保留最后输出和元数据

这样能减少对 shell 状态的依赖。

## 9. 恢复与持久化

bridge 重启时的恢复流程：

```text
启动
  -> 连接 tmux
  -> 枚举 pane
  -> 找到有 @tagb_managed=1 或 @tagb_attached=1 的 pane
  -> 读取 agent/profile/session binding 元数据
  -> 重新订阅输出
  -> 恢复本地 registry
  -> 对每个 pane 发送一份初始 snapshot 到移动端可选缓存
```

要点：

- tmux pane 是运行态真相
- 本地数据库是索引和加速层
- 两者不一致时，以 tmux 中的实际 pane 是否存在为准

## 10. 移动端交互设计

移动端不适合逐字符终端流，因此建议 UI 语义偏“远程控制面板”：

- 顶部显示当前 pane / agent / 最近活动时间
- 主要消息区显示聚合后的输出
- 快捷按钮：
  - `Snapshot`
  - `Enter`
  - `Ctrl-C`
  - `Approve`
  - `Deny`
  - `Stop`
  - `Switch`

同时应支持：

- 静默模式：只在输出稳定后推送摘要
- 观察模式：只看不发
- 控制模式：允许输入

## 11. 安全与权限模型

独立项目里，这部分比原文档更重要。

### 11.1 权限边界

bridge 本质上是在给移动端远程控制一个 tmux pane，因此要明确：

- 谁能绑定某个 pane
- 谁能向 pane 发控制输入
- 谁只能看不能控

建议最小模型：

- `viewer`：只能看 snapshot 和输出
- `operator`：可发文本、Enter、Ctrl-C
- `owner`：可 detach、kill、修改绑定

### 11.2 命令注入风险

如果移动端消息会被转成 shell 命令，风险极高。

因此要坚持：

- 普通文本输入默认只是“写到 pane stdin”
- 只有显式 `/run`、`/exec` 一类命令才允许当作外壳命令处理
- 对托管模式，尽量用 tmux 的多参数执行能力，不拼 shell 字符串

### 11.3 审计

建议记录：

- 谁在什么时间绑定了哪个 pane
- 谁发送了什么控制操作
- 谁批准了什么权限请求

## 12. 推荐的第一阶段实现范围

为了尽快验证价值，我建议把范围收窄为：

### Phase 1

- tmux control mode client
- Telegram connector
- Claude Code structured profile
- 通用 terminal relay
- 托管创建 pane
- 附着已有 pane
- snapshot / send / enter / ctrl-c / allow / deny
- 基于 tmux user options 的恢复

### Phase 2

- 飞书 connector
- Codex CLI 托管模式
- Gemini CLI 托管模式
- 多会话切换和列表卡片
- 更完整的节流与摘要

### Phase 3

- Cursor/OpenCode profile
- 文件/图片上传
- 角色权限
- Web 管理面板

## 13. 技术风险与结论

### 13.1 可行性判断

这个项目是可行的，原因是：

- tmux control mode 适合做“可观察 + 可恢复”的桥接
- 移动端主要诉求是“看”和“发控制”，不要求完整终端体验
- Claude Code 这类长驻、结构化协议明确的 agent 非常适合作为切入点

### 13.2 主要风险

1. 不同 agent 的交互模型差异很大，不能强行统一
2. 移动端消息频率低，必须有输出聚合和节流
3. 附着已有 pane 时，结构化识别能力有限，不能过度承诺
4. 如果过度依赖 shell prompt 检测，系统会很脆弱

### 13.3 最终建议

对这个独立项目，建议采用下面的设计原则：

1. tmux 是核心运行时，不是 pipe 的替代品
2. 先把“通用 terminal relay”做好，再叠加“structured agent profile”
3. 先做 Claude Code，再做其他 agent
4. 先支持“托管模式 + 明确注册的附着模式”，不要承诺接管任意历史进程

如果按这个方向推进，这份文档对应的项目目标会更清晰，工程风险也更可控。

## 14. 建议的最小项目结构

为了避免一开始就走成“大而全平台框架”，建议第一版按下面的边界拆分：

```text
tmux-agent-bridge/
├── cmd/
│   └── tagb/
│       └── main.go
├── internal/
│   ├── app/                # 启动装配、配置加载、生命周期
│   ├── config/             # TOML/YAML/ENV 配置
│   ├── bridge/             # 核心服务编排
│   ├── tmux/               # control mode client / parser / metadata
│   ├── relay/              # terminal relay、screen 聚合、snapshot
│   ├── agentprofile/       # Claude/Codex/Gemini 等 structured profile
│   ├── platform/
│   │   ├── telegram/
│   │   └── feishu/
│   ├── router/             # mobile conversation -> pane 路由
│   ├── store/              # sqlite 持久化
│   ├── permission/         # 权限请求与审批状态
│   ├── auth/               # viewer/operator/owner 权限控制
│   └── logx/               # slog 初始化、redaction
├── docs/
│   └── design-tmux-bridge.md
├── migrations/
│   └── 001_init.sql
├── scripts/
│   └── dev.sh
├── go.mod
└── README.md
```

拆分原则：

- `tmux/` 只关心 tmux 协议和目标管理
- `platform/` 只关心 Telegram/飞书 API
- `agentprofile/` 只做 structured parsing 和 structured input encoding
- `bridge/` 负责把这些模块编排起来
- `store/` 不感知 tmux 协议细节

这样可以避免未来又长成一个类似 cc-connect 的“大 core + plugin registry”。

## 15. Phase 1 核心接口草图

下面这组接口是为“尽快做出可运行版本”服务的，不追求过度抽象。

### 15.1 tmux 层

```go
type Target struct {
    Socket string // optional
    PaneID string // e.g. %5
}

type PaneInfo struct {
    Target       Target
    SessionName  string
    WindowID     string
    WindowName   string
    PaneTitle    string
    CurrentCmd   string
    Dead         bool
    Width        int
    Height       int
}

type OutputChunk struct {
    Target    Target
    Data      []byte
    ReceivedAt time.Time
}

type TmuxClient interface {
    Start(ctx context.Context) error
    Close() error

    ListPanes(ctx context.Context) ([]PaneInfo, error)
    SubscribePane(ctx context.Context, target Target) (<-chan OutputChunk, error)

    CapturePane(ctx context.Context, target Target, historyLines int) (string, error)
    InjectInput(ctx context.Context, target Target, data []byte) error
    SendKeys(ctx context.Context, target Target, keys ...string) error
    KillPane(ctx context.Context, target Target) error

    GetUserOptions(ctx context.Context, target Target) (map[string]string, error)
    SetUserOption(ctx context.Context, target Target, key, value string) error
    DeleteUserOption(ctx context.Context, target Target, key string) error
}
```

说明：

- `InjectInput` 用于写 stdin 文本流
- `SendKeys` 用于 `Enter`、`C-c` 这类控制键
- `ListPanes + user options` 是恢复基础

### 15.2 platform 层

```go
type ChatRef struct {
    Platform string // telegram / feishu
    ChatID   string
    UserID   string
}

type InboundMessage struct {
    Chat     ChatRef
    Text     string
    Files    []PlatformFile
    MessageID string
}

type Action struct {
    Chat      ChatRef
    ActionID  string
    Value     string
    MessageID string
}

type PlatformConnector interface {
    Name() string
    Start(ctx context.Context, h PlatformHandler) error
    SendText(ctx context.Context, chat ChatRef, text string) error
    SendCard(ctx context.Context, chat ChatRef, card Card) error
    EditMessage(ctx context.Context, chat ChatRef, messageID string, text string) error
}

type PlatformHandler interface {
    OnMessage(ctx context.Context, msg InboundMessage)
    OnAction(ctx context.Context, act Action)
}
```

### 15.3 bridge 层

```go
type Bridge interface {
    Start(ctx context.Context) error
    BindChatToPane(ctx context.Context, chat ChatRef, target Target, mode BindMode) error
    AttachPane(ctx context.Context, target Target, meta AttachMeta) error
    HandleInboundMessage(ctx context.Context, msg InboundMessage) error
    HandleAction(ctx context.Context, act Action) error
}

type BindMode string

const (
    BindView    BindMode = "view"
    BindOperate BindMode = "operate"
)
```

### 15.4 structured profile 层

```go
type BridgeEventType string

const (
    EventTerminal   BridgeEventType = "terminal"
    EventText       BridgeEventType = "text"
    EventThinking   BridgeEventType = "thinking"
    EventToolUse    BridgeEventType = "tool_use"
    EventPermission BridgeEventType = "permission"
    EventResult     BridgeEventType = "result"
    EventError      BridgeEventType = "error"
)

type BridgeEvent struct {
    Type         BridgeEventType
    Target       Target
    Content      string
    RequestID    string
    ToolName     string
    ToolInput    string
    SessionID    string
    Raw          map[string]any
    Done         bool
    OccurredAt   time.Time
}

type AgentProfile interface {
    Name() string
    Detect(meta map[string]string, snapshot string) bool
    ParseLine(line []byte) ([]BridgeEvent, error)
    EncodeUserMessage(text string) ([]byte, error)
    EncodePermissionDecision(requestID string, allow bool) ([]byte, error)
}
```

这里要注意一件事：

- `AgentProfile` 只面向“单行结构化输入/输出协议”
- 文件上传、图片上传可以在 profile 外单独加 capability，Phase 1 先不做

## 16. Phase 1 状态机

第一版真正需要跑通的状态其实不多。不要一开始做复杂 workflow engine。

### 16.1 Pane 生命周期状态

```text
discovered
  -> attached
  -> bound
  -> active
  -> idle
  -> closed
```

定义：

- `discovered`：bridge 在 tmux 中发现 pane，但尚未注册
- `attached`：pane 已被 bridge 接管，写入了元数据
- `bound`：已有移动端会话绑定到该 pane
- `active`：最近有输出或输入活动
- `idle`：已绑定但空闲
- `closed`：pane 不存在或已显式 detach

### 16.2 移动端消息处理状态机

```text
idle
  -> routing
  -> injecting
  -> waiting_output
  -> streaming
  -> settled
```

建议语义：

1. `routing`
   - 找到当前 chat 绑定的 pane
   - 校验权限
2. `injecting`
   - 根据 relay mode / structured mode 注入输入
3. `waiting_output`
   - 等待第一段输出或超时
4. `streaming`
   - 持续接收 chunk，并交给 ScreenService 聚合
5. `settled`
   - 输出在一定时间窗口内静止，发送最终摘要或结束标记

这里的关键不是“agent 是否真正结束”，而是“移动端这轮交互是否已经稳定”。

### 16.3 权限请求状态机

```text
none
  -> pending
  -> approved | denied | expired
```

规则：

- 同一 pane 同一时刻允许多个 pending request，但移动端展示要按时间顺序
- request 必须关联 `pane_id + request_id`
- 超时后默认 `expired`
- 是否自动 deny，要做成配置项

## 17. 输出聚合策略

这是移动端体验的关键，建议在文档里先把规则定住。

### 17.1 为什么必须聚合

tmux `%output` 是终端字节流，不适合直接推给 Telegram/飞书：

- 粒度太碎
- 会包含回车、局部刷新、prompt 回显
- 平台消息有频率限制

### 17.2 Phase 1 聚合规则

建议：

1. 先做 raw bytes -> line buffer
2. structured profile 命中时，优先发结构化事件
3. 未命中时，按 terminal 文本块聚合

默认窗口：

- `flush_interval = 500ms`
- `settle_after = 2s`
- `max_chunk_chars = 1200`

输出策略：

- terminal relay：发代码块文本
- thinking/tool/text：分别映射为不同的消息前缀
- result：尽量单独成消息
- error：立即发送，不等聚合窗口

### 17.3 Snapshot 策略

`/snapshot` 不应该只抓当前可见 24 行，建议支持：

- `recent screen`：当前屏幕
- `history 200`：最近 200 行
- `history 1000`：最近 1000 行，必要时截断

移动端展示上：

- 默认发最近 80 到 120 行
- 超长内容上传为文本文件或分片发送

## 18. tmux metadata 约定

为了让恢复和附着行为稳定，建议尽早定下 metadata key。

### 18.1 必需字段

```text
@tagb_managed=1
@tagb_mode=relay|structured
@tagb_agent=claude|codex|gemini|cursor|unknown
@tagb_bound_platform=telegram
@tagb_bound_chat_id=123456
@tagb_bound_user_id=987654
@tagb_role=viewer|operator|owner
@tagb_last_activity_unix=1710000000
```

### 18.2 可选字段

```text
@tagb_agent_session_id=...
@tagb_permission_pending=0|1
@tagb_profile_version=1
@tagb_label=backend-api
@tagb_created_by=bridge|manual-attach
```

规则：

- 必需字段由 bridge 写入和维护
- 可选字段用于加速恢复和 UI 展示
- 任何缺失字段都不能导致 bridge 崩溃，只能降级

## 19. 本地持久化设计

建议第一版直接用 SQLite。理由：

- 单机守护进程足够
- 易于排查
- 适合做审计和消息映射

### 19.1 表结构建议

```sql
create table panes (
  pane_key text primary key,
  socket_name text not null default '',
  pane_id text not null,
  session_name text not null default '',
  window_name text not null default '',
  agent text not null default 'unknown',
  mode text not null default 'relay',
  label text not null default '',
  last_activity_unix integer not null default 0,
  last_snapshot text not null default '',
  created_at_unix integer not null,
  updated_at_unix integer not null
);

create table bindings (
  id integer primary key autoincrement,
  platform text not null,
  chat_id text not null,
  user_id text not null default '',
  pane_key text not null,
  role text not null default 'operator',
  active integer not null default 1,
  created_at_unix integer not null,
  updated_at_unix integer not null
);

create table permissions (
  id integer primary key autoincrement,
  pane_key text not null,
  request_id text not null,
  tool_name text not null default '',
  tool_input text not null default '',
  status text not null default 'pending',
  platform text not null default '',
  chat_id text not null default '',
  created_at_unix integer not null,
  updated_at_unix integer not null
);

create table audit_logs (
  id integer primary key autoincrement,
  pane_key text not null default '',
  platform text not null default '',
  chat_id text not null default '',
  actor text not null default '',
  action text not null,
  payload_json text not null default '',
  created_at_unix integer not null
);
```

### 19.2 主键约定

建议 `pane_key` 统一为：

```text
<socket_name>:<pane_id>
```

例如：

```text
default:%5
main:%12
```

这样比仅存 `%5` 更安全，因为 tmux 多 socket 场景迟早会遇到。

## 20. 新仓库初始化顺序

如果现在就开干，我建议按下面顺序建立仓库。

### Step 1

- 初始化 Go 项目
- 搭好 `cmd/tagb`、`internal/config`、`internal/logx`
- 先让守护进程能启动并连上 tmux

### Step 2

- 实现 `internal/tmux`
- 跑通：
  - `list-panes`
  - `capture-pane`
  - `send-keys`
  - `InjectInput`
  - user option 读写

### Step 3

- 实现 `internal/store` 和 migration
- 建立 pane registry
- 支持 attach / detach / recover

### Step 4

- 实现 Telegram connector
- 跑通：
  - `/list`
  - `/bind`
  - `/snapshot`
  - 普通消息 -> pane 输入
  - `/enter`
  - `/ctrl-c`

### Step 5

- 实现 Claude structured profile
- 跑通：
  - text/thinking/result
  - permission request
  - allow/deny

### Step 6

- 增加输出节流和卡片/按钮
- 补审计、权限角色
- 再开始支持 Codex/Gemini

## 21. Phase 1 验收标准

满足下面这些，第一版就算成立：

1. 用户能把一个现有 tmux pane 绑定到 Telegram 会话
2. 用户能在手机上看到该 pane 的持续输出和 snapshot
3. 用户能从手机向该 pane 发送文本、回车、Ctrl-C
4. bridge 重启后能恢复绑定和输出订阅
5. Claude Code 在 structured mode 下能正确处理权限请求
6. 对无法结构化识别的 pane，系统仍能以 relay mode 正常工作
