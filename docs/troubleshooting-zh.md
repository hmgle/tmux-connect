# tmux-connect 中文故障排查

## 先做这三个检查

```bash
./tmux-connect daemon doctor --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
./tmux-connect list
```

这三个命令通常足够先判断问题在：

- Telegram token / 网络
- SQLite
- tmux pane 可见性

## 常见问题

### daemon 启动失败

常见原因：

- `sqlite3` 不在 `PATH`
- Telegram token 无效
- 无法访问 `api.telegram.org`
- `--db` 路径不可写

建议先执行：

```bash
./tmux-connect daemon doctor --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"
```

### Telegram 命令无响应

优先检查：

- 当前聊天是否在 `--allow-chat` 白名单中
- daemon 进程是否还活着
- 服务器是否能访问 Telegram Bot API
- token 是否正确
- 是否有另一台机器也在用同一个 bot token 运行 `tmux-connect daemon run`

补充说明：

- 当前版本要求同一个 bot token 只有一个活跃轮询实例
- 如果要管理多台机器，建议每台机器使用不同的 bot

### `/select` 失败

常见原因：

- pane ID 写错了
- pane 已经不存在
- 当前 tmux socket 不对

建议：

```bash
./tmux-connect list
./tmux-connect inspect --pane %5
```

### `/snapshot` 失败

常见原因：

- pane 已消失
- 当前聊天没有已选择的 pane

先检查：

```text
/current
/panes
```

### `/follow on` 后没有输出

这不一定是故障，也可能只是 pane 当前没有新输出。

建议按顺序排查：

1. `/snapshot` 看当前内容
2. 确认 Agent 是否在等待输入
3. 先直接发送命令文本，再补一个 `/enter`，或者直接用 `/enter <命令>`
4. 如果 daemon 刚重启过，重新执行一次 `/follow on`

### daemon 重启后 follow 丢失

这是当前设计行为，不是 bug。

`follow` 是内存态，不会跨 daemon 重启恢复。重启后重新执行：

```text
/follow on
```

### pane 被关闭以后会怎样

当前 pane 消失后：

- daemon 会在后续操作中发现 pane 不可用
- 自动清除相关聊天的当前 pane 状态
- 要求用户重新 `/select`

### 图片快照渲染异常

常见原因：

- 自定义字体文件不存在
- 字体文件不是 `.ttf` 或 `.otf`
- 字体损坏

可以先移除自定义字体参数，回退到内置 `gomono` 字体。

## 退出码

| 退出码 | 含义 |
|------|------|
| `0` | 成功 |
| `2` | 输入参数无效 |
| `3` | pane 未找到或 pane 操作失败 |
| `4` | tmux 通信失败 |

## 需要继续看的文档

- [guide-zh.md](./guide-zh.md) — 中文快速开始
- [telegram.md](./telegram.md) — Telegram 配置和命令说明
- [slack.md](./slack.md) — Slack 配置和命令说明
- [discord.md](./discord.md) — Discord 配置和命令说明
- [whatsapp.md](./whatsapp.md) — WhatsApp 配置和命令说明
- [architecture.md](./architecture.md) — 当前架构与恢复模型
