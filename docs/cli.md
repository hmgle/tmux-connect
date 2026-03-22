# CLI Reference

## Configuration

Configuration can be loaded from `--config PATH` or, by default,
`$XDG_CONFIG_HOME/tmux-connect/config.toml` (falling back to
`$HOME/.config/tmux-connect/config.toml`). Precedence:

1. Command-line flags (highest)
2. Environment variables
3. TOML config file (lowest)

Global flags (`--config`, `--socket`, `--json`) must appear before the subcommand.

## Global Flags

| Flag | Description |
|------|-------------|
| `--config PATH` | Load TOML configuration file |
| `--socket NAME` / `-L NAME` | Tmux socket name |
| `--json` | Machine-readable JSON output |

## Commands

### list

List all tmux panes with bridge metadata.

```bash
./tmux-connect list
./tmux-connect --json list
```

### attach

Attach an existing pane to the bridge.

```bash
./tmux-connect attach --pane %5 --agent codex --label backend
```

| Flag | Default | Description |
|------|---------|-------------|
| `--pane` | required | Target pane ID |
| `--agent` | `unknown` | Agent identifier (`codex`, `claude`, etc.) |
| `--label` | `""` | Human-readable label |

### detach

Remove a pane from the bridge.

```bash
./tmux-connect detach --pane %5
```

### inspect

View detailed pane metadata and bridge state.

```bash
./tmux-connect inspect --pane %5
```

### snapshot

Capture recent terminal output from a pane.

```bash
./tmux-connect snapshot --pane %5 --lines 120
```

| Flag | Default | Description |
|------|---------|-------------|
| `--pane` | required | Target pane ID |
| `--lines` | `120` | Number of lines to capture |

### stream

Follow pane output in real time. Prefers tmux control mode, falls back to polling.

```bash
./tmux-connect stream --pane %5
./tmux-connect --json stream --pane %5 --lines 80
```

| Flag | Default | Description |
|------|---------|-------------|
| `--pane` | required | Target pane ID |
| `--lines` | `120` | Initial lines to include |

### send

Inject text into a pane, optionally pressing Enter.

```bash
./tmux-connect send --pane %5 --text "make test" --enter
./tmux-connect send --pane %5 --text "continue"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--pane` | required | Target pane ID |
| `--text` | required | Text to send |
| `--enter` | `false` | Press Enter after text |

### enter

Send an Enter key to a pane.

```bash
./tmux-connect enter --pane %5
```

### ctrl-c

Send Ctrl+C to a pane.

```bash
./tmux-connect ctrl-c --pane %5
```

### serve

Start the HTTP API server. See [api.md](./api.md) for endpoint details.

```bash
./tmux-connect serve --listen 127.0.0.1:8080
```

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `127.0.0.1:8080` | Listen address |

### daemon

Run the relay daemon. See [daemon.md](./daemon.md) for full configuration.

```bash
./tmux-connect daemon run [flags]
./tmux-connect daemon doctor [flags]
./tmux-connect daemon status [flags]
```

## Full Syntax

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

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Unexpected error |
| `2` | Invalid input |
| `3` | Pane not found |
| `4` | Tmux communication error |
