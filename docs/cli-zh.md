# CLI 命令参考

## 配置

配置可以通过 `--config PATH` 指定，默认读取
`$XDG_CONFIG_HOME/tmux-connect/config.toml`，若未设置则回退到
`$HOME/.config/tmux-connect/config.toml`。优先级：

1. 命令行参数（最高）
2. 环境变量
3. TOML 配置文件（最低）

全局参数（`--config`、`--socket`、`--json`）必须放在子命令之前。

## 全局参数

| 参数 | 说明 |
|------|------|
| `--config PATH` | 加载 TOML 配置文件 |
| `--socket NAME` / `-L NAME` | tmux socket 名称 |
| `--json` | 输出机器可读的 JSON |

## 命令

### list

列出所有 tmux pane 及 bridge 元数据。

```bash
./tmux-connect list
./tmux-connect --json list
```

### attach

将现有 pane 接入 bridge。

```bash
./tmux-connect attach --pane %5 --agent codex --label backend
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--pane` | 必填 | 目标 pane ID |
| `--agent` | `unknown` | Agent 标识（`codex`、`claude` 等） |
| `--label` | `""` | 可读标签 |

### detach

将 pane 从 bridge 移除。

```bash
./tmux-connect detach --pane %5
```

### inspect

查看 pane 详细元数据和 bridge 状态。

```bash
./tmux-connect inspect --pane %5
```

### snapshot

抓取 pane 最近的终端输出。

```bash
./tmux-connect snapshot --pane %5 --lines 120
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--pane` | 必填 | 目标 pane ID |
| `--lines` | `120` | 抓取行数 |

### stream

实时跟随 pane 输出。优先使用 tmux control mode，回退到轮询。

```bash
./tmux-connect stream --pane %5
./tmux-connect --json stream --pane %5 --lines 80
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--pane` | 必填 | 目标 pane ID |
| `--lines` | `120` | 初始包含行数 |

### send

向 pane 注入文本，可选是否按回车。

```bash
./tmux-connect send --pane %5 --text "make test" --enter
./tmux-connect send --pane %5 --text "continue"
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--pane` | 必填 | 目标 pane ID |
| `--text` | 必填 | 要发送的文本 |
| `--enter` | `false` | 发送后按回车 |

### enter

向 pane 发送回车键。

```bash
./tmux-connect enter --pane %5
```

### ctrl-c

向 pane 发送 Ctrl+C。

```bash
./tmux-connect ctrl-c --pane %5
```

### serve

启动 HTTP API 服务。端点详情见 [api-zh.md](./api-zh.md)。

```bash
./tmux-connect serve --listen 127.0.0.1:8080
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--listen` | `127.0.0.1:8080` | 监听地址 |

### daemon

运行中继 daemon。完整配置见 [daemon-zh.md](./daemon-zh.md)。

```bash
./tmux-connect daemon run [flags]
./tmux-connect daemon doctor [flags]
./tmux-connect daemon status [flags]
```

## 完整语法

```
./tmux-connect [--config PATH] [--socket NAME] [--json] list
./tmux-connect [--config PATH] [--socket NAME] [--json] attach --pane ID [--agent NAME] [--label NAME]
./tmux-connect [--config PATH] [--socket NAME] [--json] detach --pane ID
./tmux-connect [--config PATH] [--socket NAME] [--json] inspect --pane ID
./tmux-connect [--config PATH] [--socket NAME] [--json] snapshot --pane ID [--lines N]
./tmux-connect [--config PATH] [--socket NAME] [--json] stream --pane ID [--lines N]
./tmux-connect [--config PATH] [--socket NAME] [--json] send --pane ID --text TEXT [--enter]
./tmux-connect [--config PATH] [--socket NAME] [--json] enter --pane ID
./tmux-connect [--config PATH] [--socket NAME] [--json] ctrl-c --pane ID
./tmux-connect [--config PATH] [--socket NAME] serve [--listen ADDR]
./tmux-connect [--config PATH] [--socket NAME] daemon <run|doctor|status> [flags]
```

## 退出码

| 代码 | 含义 |
|------|------|
| `0` | 成功 |
| `1` | 未预期的错误 |
| `2` | 输入无效 |
| `3` | Pane 未找到 |
| `4` | tmux 通信错误 |
