# Discord Setup and Operations

## Prerequisites

- `tmux` is installed and the target pane already exists
- `sqlite3` is installed and available in `PATH`
- you have a Discord application with a bot token
- the bot is invited to the target server or DM
- the bot has the Message Content intent enabled if you want prefixed commands in channels or DMs

## Discord Application Setup

- create a bot user in the Discord developer portal
- copy the bot token
- enable:
  - `MESSAGE CONTENT INTENT`
- invite the bot with permissions to:
  - view channels
  - send messages
  - attach files
  - create public threads if you plan to use it in channels with threads

Slash commands are registered globally by the daemon at startup. Discord may take a short time to show global command updates.

## Start the Daemon

`tmux-connect` also reads `$XDG_CONFIG_HOME/tmux-connect/config.toml` by default
(falling back to `$HOME/.config/tmux-connect/config.toml`). Flags override
environment variables, and environment variables override the TOML file.

Using TOML config:

```toml
[daemon]
platform = "discord"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["discord:123456789012345678"]
snapshot_lines = 120
follow_lines = 80
follow_min_interval = "700ms"

[daemon.discord]
token = "discord-bot-token"
command_prefix = "tmux:"
```

```bash
go run ./cmd/tmux-connect daemon run
```

Using explicit flags:

```bash
go run ./cmd/tmux-connect daemon run \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN" \
  --discord-command-prefix "tmux:" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat discord:123456789012345678
```

Useful flags:

- `--platform discord`
- `--discord-token TOKEN`
- `--discord-command-prefix PREFIX` (default: `tmux:`)
- `--db PATH`
- `--allow-chat discord:CHANNEL_OR_DM_ID`
- `--snapshot-lines 120`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`

Supported environment variables:

- `TMUXCONN_PLATFORM=discord`
- `TMUXCONN_DISCORD_TOKEN`
- `TMUXCONN_DISCORD_COMMAND_PREFIX`
- `TMUXCONN_DB_PATH`
- `TMUXCONN_FOLLOW_DEBUG`

## Health Checks

```bash
go run ./cmd/tmux-connect daemon doctor \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN"

go run ./cmd/tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

`daemon doctor` confirms the token is present and reminds you to enable the Message Content intent for prefix commands and DMs.

## Using the Bot

- use slash commands such as `/panes`, `/select`, `/snapshot`, or `/follow`
- in guild channels, you can also use prefixed commands such as `tmux: panes`
- in DMs, plain text sends directly to the current pane without pressing Enter
- in channels, plain text is ignored unless it is a slash command or prefixed command
- when a command prompts for more input in a channel, keep using the configured prefix, for example `tmux: %5`

Supported commands:

- `/start`
- `/help`
- `/panes`
- `/select <pane>`
- `/clear`
- `/unmanage <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>` for explicit text sends, especially if the text itself starts with `/`
- `/keys <key...>` or `/key <key...>` for tmux key names such as `C-c`, `PageUp`, `F1`, or `M-x`
- `/enter [text]`
- `/ctrlc` or `/ctrl-c`
- `/follow on [interval]|off`

Prefixed channel equivalents:

- `tmux: panes`
- `tmux: select <pane>`
- `tmux: current`
- `tmux: snapshot [lines] [image|text]`
- `tmux: send <text>`
- `tmux: keys <key...>`
- `tmux: enter [text]`
- `tmux: ctrlc`
- `tmux: follow on [interval]|off`

## Operational Notes

- slash commands are acknowledged immediately and the daemon edits the original interaction response when it has the result
- Discord channel replies stay scoped to the same channel conversation
- DMs treat plain text as pane input; use `/enter` to execute after reviewing text
- snapshot defaults to image when rendering succeeds; text mode is still available
- current pane bindings and message continuity survive daemon restarts through SQLite
- allowlists should prefer `discord:<channel_id>` or `discord:<dm_id>` entries to avoid collisions with other platforms
