# tmux / daemon 优化计划

## 目标

这份文档汇总了对当前项目 tmux 层、daemon 路由层和 SQLite 持久化层的优化建议，并吸收了对 `../libtmux/` 的对比结论。

结论先行：

- 可以借鉴 `libtmux`，但不建议照搬它的 `Server/Session/Window/Pane` 富对象模型。
- 最值得借的是三件事：
  - 命令执行层与业务层分离。
  - `list-* -F` 的结构化抓取与统一解析。
  - 明确的能力探测和错误分类。
- 当前项目最优先的问题，不是 API 风格，而是并发安全、子进程生命周期、命令次数和热路径上的重复查询。

## 进展同步

截至当前工作树，这些优化已经完成并提交：

- `e61f906` `feat: optimize tmux metadata and daemon persistence paths`
  - 修复 `InjectInput()` buffer 名冲突。
  - control-mode PTY 生命周期回收。
  - metadata 批量写入。
  - ReplyBus / Store 的 SQLite 事务式批处理。
- `f367e91` `feat: reduce targeted pane lookup overhead`
  - 单 pane 操作改为目标化查询。
  - daemon managed fast path，减少 `TouchMetadata()` 冗余读取。
- `0062402` `feat: cache unsupported tmux capabilities`
  - control-mode / rich capture 的不支持能力缓存。
- `cd4760c` `feat: defer registry refresh until needed`
  - `PaneRegistry` dirty-flag。
  - `/attach`、`/detach` 不再立刻全量 refresh。
- `52f9116` `feat: classify tmux option errors`
  - 引入 `ErrTmuxOptionUnavailable`，收敛 option 不可用错误判断。
- `ced889b` `feat: add structured tmux runner results`
  - runner 返回结构化 `stdout/stderr/exitCode/args`。
  - 错误分类优先使用结构化结果，字符串匹配仅作 fallback。

尚未完成的重点项：

- pane schema 的统一 descriptor 化。
- 更完整的 typed error 体系，不止 option/control 两类。
- registry 作为 `requireCurrentPane()` 的快速预检。
- 是否需要更显式的 tmux 启动自检入口。

## 评估总表

| ID | 项目 | 结论 | 说明 |
| --- | --- | --- | --- |
| A1 | `SetMetadata` / `ClearMetadata` 多次 `set-option` | 成立 | 现在是 6 次独立 exec，可以合并为 1 次 tmux 命令链，显著减少 fork/exec。 |
| A2 | `TouchMetadata` 读后写 | 部分成立 | 当前实现确实是 2 次 exec；在 daemon 已确认 managed 的热路径可降为 1 次，但 CLI/通用 service 不能无条件跳过读取。 |
| A3 | `startControlSubscription()` 里全局 `ListPanes()` | 成立 | 这里只需要当前 session 的 pane 集合，不需要全局扫描。 |
| A4 | ReplyBus SQLite 写入缺少批处理事务 | 成立 | `recordOutbound()` 和 `LogInbound()` 都会触发多次独立 sqlite3 进程调用，适合合并事务。 |
| A5 | `ResolvePane()` 每次全量扫描 pane | 成立 | 目前大多数单 pane 操作都会先 `ListPanes()` 再执行目标命令。 |
| A6 | Registry 刷新太频繁 | 部分成立 | `/panes`、`/attach`、`/detach` 会显式刷新；优化空间存在，但频率并不算极端，收益中等。 |
| A7 | 缺少 tmux 版本 / 能力检测 | 成立 | 当前没有统一的版本探测或 capability probe。 |
| A8 | format string 与 struct 手工同步 | 成立 | 当前字段协议分散在 format、parser、builder 多处，扩展时容易失配。 |
| A9 | 错误类型不够结构化 | 成立 | 目前依赖字符串匹配和通用 error，适合补 sentinel/typed error。 |
| A10 | `handleSnapshot` 总是双重 capture | 不成立 | `text` 模式下已直接返回纯文本 snapshot；只有 image 默认模式才会再尝试 rich capture。 |
| B1 | `InjectInput()` buffer 名冲突 | 成立 | 同一 pane 并发发送时会复用 `tagb-<pane>`，存在覆盖风险。 |
| B2 | control-mode PTY 未 `Wait()` 回收 | 成立 | 关闭订阅时没有回收 `attach-session` 子进程，长期运行有泄漏风险。 |
| B3 | control-mode 失败后无差别降级 polling | 成立 | 会掩盖真实 bug、版本差异或协议问题，不利于诊断。 |
| B4 | 需要引入完整 libtmux 对象模型 | 不成立 | 当前项目是 pane-first 服务，不需要为 ergonomics 引入富对象树。 |

## 已确认的优化点

## 1. tmux 热路径减少命令次数

### 1.1 `SetMetadata()` / `ClearMetadata()` 合并为单次 tmux 调用

状态：已完成

当前：

- `SetMetadata()` 遍历 `meta.ToOptions()`，每个字段单独调用一次 `set-option`。
- `ClearMetadata()` 逐个字段 `set-option -u`。

问题：

- 元数据写入一次 attach 会触发多次 fork/exec。
- `SetMetadata()` 还依赖 map 迭代顺序；失败时可能产生“部分写入”的中间状态。

建议：

- 新增一个批量命令构建器，把多条 `set-option` 通过 tmux `\;` 链接后一次执行。
- 元数据字段顺序固定化，不再依赖 map 迭代。

预期收益：

- attach/detach 之类的元数据写入从 6 次 tmux exec 降到 1 次。
- 代码语义更清晰，测试更容易覆盖。

### 1.2 `TouchMetadata()` 增加 fast path

状态：已完成

当前：

- 先 `show-options -p -v @tagb_managed`
- 再 `set-option -p @tagb_last_activity_unix`

问题：

- `Send()`、`Enter()`、`CtrlC()` 都会触发这一逻辑。
- daemon 路由中的 `requireCurrentPane()` 已经确认 pane 是 managed，但 service 仍重复读取。

建议：

- 保留当前 `TouchMetadata()` 作为安全默认路径。
- 新增 `TouchMetadataManaged()` 或 `TouchMetadataIfKnownManaged(managed bool)`。
- daemon 热路径在已确认 managed 后直接走单写入路径。

注意：

- 这个优化只适合“调用方已验证 pane 受管”的路径。
- CLI 和其他通用入口仍应保守处理。

### 1.3 `ResolvePane()` 降低全量扫描依赖

状态：已部分完成

当前：

- `Inspect()`、`Snapshot()`、`Send()`、`Enter()`、`CtrlC()`、`OpenStream()` 都会先 `ResolvePane()`。
- `ResolvePane()` 内部调用 `ListPanes()`，对全局 pane 做扫描匹配。

问题：

- 单 pane 操作经常变成“先全局 list，再单独命令”。
- daemon 高交互路径会放大 tmux 往返次数。

建议：

- 当前已完成：
  - `GetPane()` / `GetPaneState()` 目标化查询。
  - `Inspect()` 单次取回 pane 信息和 metadata。
  - managed 路径减少不必要的额外读取。
- 后续可选：
  - 评估是否还需要显式 `PaneRef` / `ResolvedPane` 类型。

注意：

- 不能完全移除 authoritative check；pane 可能已经不存在。
- 更合理的方向是“减少全量 list”，不是“完全不校验”。

## 2. control-mode 稳定性与生命周期

### 2.1 回收 PTY 子进程

状态：已完成

当前：

- `StartPTY()` 返回的 `PTYSession` 暴露了 `Wait()`。
- `startControlSubscription()` 关闭时只做 `detach-client` 和 `Close()`，没有 `Wait()`。
- `attach-session` 启动时使用的是父 `ctx`，不是内部的 `subCtx`。

问题：

- 关闭订阅后可能留下未回收子进程。
- 长时间 daemon/follow 运行下存在资源泄漏风险。

建议：

- 启动 PTY 时改用 `subCtx`。
- 关闭时显式等待子进程退出。
- 增加进程关闭、超时、重复关闭场景的测试。

### 2.2 缩小 control-mode 初始化范围

状态：已完成

当前：

- `startControlSubscription()` 会先 `ListPanes()`，再筛选 `SessionName` 相同的 pane。

问题：

- 为了初始化一个 session 内的流订阅，做了全局 pane 查询。

建议：

- 新增 session-scoped pane listing，比如 `list-panes -t <session>`。
- 保持 control 初始化只关注目标 session。

### 2.3 区分“可降级错误”和“必须暴露错误”

状态：已完成

当前：

- `OpenPaneStream()` / `SubscribePane()` 只要 control-mode 失败就直接退回 polling。

问题：

- 会把协议 bug、实现 bug、tmux 版本差异和暂时性故障混为一类。
- 问题容易被掩盖，长期依赖 polling。

建议：

- 定义控制流错误分类，例如：
  - `ErrControlUnsupported`
  - `ErrControlHandshakeTimeout`
  - `ErrControlProtocol`
- 只有“明确不支持”才自动降级。
- 其他错误应记录并暴露，至少在日志或诊断里可见。

## 3. SQLite 热路径批处理

### 3.1 ReplyBus 出站写入合并事务

状态：已完成

当前：

- `recordOutbound()` 会顺序调用：
  - `LogMessage()`
  - `TouchSessionOutbound()`
  - `CreateMessageLink()`

问题：

- 每次都是独立 sqlite3 进程调用。
- 虽然 `Store` 内部有 mutex 串行化，但仍然有多次进程启动开销。

建议：

- 新增批量 API，例如 `RecordOutbound()`。
- 把 3 条 SQL 合并成一个事务脚本，单次 `sqlite3` 调用完成。

### 3.2 入站写入也做同样合并

状态：已完成

当前：

- `LogInbound()` 会调用：
  - `LogMessage()`
  - `EnsureSession()`
  - `TouchSessionInbound()`
  - `CreateMessageLink()`

建议：

- 新增 `RecordInbound()` 事务入口。
- 把 session ensure、touch 和 message link 合并在一个事务里。

注意：

- `EnsureSession()` 当前对 `RETURNING` 有兼容 fallback，合并时需要保留兼容逻辑。
- 这里的目标是减少 sqlite3 进程数，不是把 `Store` 改成复杂 ORM。

## 4. registry 与 daemon 路由

### 4.1 `PaneRegistry` 可做延迟刷新，但收益中等

状态：已完成

当前：

- `/panes`、`/attach`、`/detach` 会显式调用 `registry.Refresh()`。
- `requireCurrentPane()` 不使用 registry，而是走 `service.Inspect()`。

判断：

- “刷新太频繁”有一定道理，但不是当前最重的热点。
- 它不像 tmux exec 风险和子进程泄漏那么高优先级。

建议：

- 可引入 dirty flag：
  - 写操作后只标记 dirty。
  - 下次读取列表时再刷新。
- 但不要为了这件事牺牲状态正确性。

### 4.2 `requireCurrentPane()` 参考 registry 只能算部分优化

状态：未开始

判断：

- registry 的确是缓存，但它可能过期。
- `requireCurrentPane()` 目前除了确认 pane 存在，还要确认仍是 managed。

建议：

- registry 可以作为快速预检：
  - 先判断 key 是否已知。
  - 快速返回更友好的错误。
- 最终仍应保留一次 authoritative 校验，避免因缓存陈旧导致误判。

## 5. 输出协议、命令层和错误模型

### 5.1 收敛 tmux list schema

状态：未开始

当前：

- `paneListFormat()`
- `paneStateFormat()`
- `parsePaneInfoLine()`
- `parsePaneStateLine()`
- `buildPaneInfo()`

问题：

- 字段顺序与字段数必须手动同步。
- 扩字段时容易出现 format 和 parser 脱节。

建议：

- 把 pane schema 收敛成一份描述表。
- 由 schema 生成 format string，并复用解析逻辑。
- 不一定需要反射；简单 descriptor 就足够。

说明：

- 这里可以借鉴 `libtmux` 的 `neo.get_output_format()` 和 `fetch_objs()` 思路。
- 但不需要引入它的完整 dataclass introspection 方案。

### 5.2 命令执行结果结构化

状态：已完成

当前：

- `RealRunner.Run()` 只返回字符串和 error。
- 上层很多逻辑只能通过 `err.Error()` 做字符串判断。

建议：

- 把 runner 返回值升级为结构化结果：
  - `stdout`
  - `stderr`
  - `exitCode`
  - `argv`
- 基于结果定义 typed/sentinel error，而不是散落的字符串匹配。

### 5.3 增加能力探测

状态：已部分完成

当前：

- 没有统一的 tmux 版本或 capability 检测。

建议：

- 当前已完成：
  - control-mode unsupported 缓存。
  - rich capture unsupported 缓存。
- 后续可选：
  - 增加更显式的 startup doctor / capability probe。
  - 把 tmux 存在性、socket 可达性纳入统一入口。

## 6. 并发安全

### 6.1 `InjectInput()` buffer 命名改为唯一值

状态：已完成

当前：

- buffer 名是 `tagb-<paneID>`。

问题：

- 同一 pane 并发发送时，多个请求会竞争同一个 tmux buffer。

建议：

- buffer 名加入唯一后缀，例如时间戳、原子递增序号或随机值。
- 保持 `paste-buffer -d` 删除 buffer，避免残留。

## 不建议做的事情

- 不要照搬 `libtmux` 的 `Server/Session/Window/Pane` 富对象模型。
- 不要引入类似 `QueryList` 的 DSL 过滤层。
- 不要为了减少一次 list 而彻底取消 pane 存在性校验。

## 已经做得不错的部分

- `InjectInput()` 采用 `load-buffer + paste-buffer`，比纯 `send-keys -l` 更稳。
- `ListPaneStates()` 已经把 metadata 合并进一次 `list-panes -F` 查询，方向是对的。
- control-mode 和 polling 双模降级是有价值的，只是错误分类还需要补强。
- `CutoverSubscription()` 的 snapshot 到 stream 去重逻辑做得不错。
- `DeleteUserOption()` 已经正确处理 option 不存在场景。

## 建议执行顺序

## P0

状态：已完成

- 修复 `InjectInput()` buffer 名冲突。
- 修复 control-mode PTY 生命周期和 `Wait()` 回收。
- 增加 control-mode 错误分类，限制“无差别降级”。

## P1

状态：已大体完成

- 已完成：
  - 合并 `SetMetadata()` / `ClearMetadata()` 为单次 tmux 调用。
  - 为 `TouchMetadata()` 增加 managed fast path。
  - 把 `startControlSubscription()` 的 pane查询改为 session-scoped。
  - 把 ReplyBus 的入站/出站存储合并为事务式单次 sqlite3 调用。

## P2

状态：部分完成

- 已完成：
  - 单 pane 目标化查询，减少对全量 `ListPanes()` 的依赖。
  - 结构化 runner result。
  - option/control/capability 相关的一部分 typed error / sentinel error。
  - runtime unsupported capability caching。
- 未完成：
  - pane schema 的统一 descriptor。
  - 更完整的一般化 tmux typed error。
  - 是否要引入显式 `PaneRef` 类型，仍待评估。

## P3

状态：部分完成

- 已完成：
  - registry dirty-flag / 延迟刷新。
- 未完成：
  - registry 作为 `requireCurrentPane()` 的快速预检。

## 测试补充建议

- `InjectInput()` 并发发送不会互相覆盖。
- control subscription 关闭后子进程被正确回收。
- control-mode handshake 超时、协议错误、明确不支持三类错误可区分。
- 批量 metadata 写入只产生一次 tmux 调用。
- inbound/outbound 事务写入只产生一次 sqlite3 调用。
- session-scoped pane 查询不会影响现有 follow/stream 行为。

## 备注

`handleSnapshot` 的“双重 capture”不应列为优化项。当前逻辑在 `text` 模式下只调用 `Snapshot()`；只有默认 `image` 模式才会额外尝试 `SnapshotRich()`，这是功能需要，不是无条件重复工作。
