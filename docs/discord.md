# Discord Setup and Operations

## Prerequisites

- `tmux` is installed and the target pane already exists
- no external `sqlite3` CLI is required; the daemon uses embedded SQLite
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
# optional allowlist; remove this line to allow any reachable Discord channel or DM
allow_chats = ["discord:123456789012345678"]
snapshot_lines = 120
plain_text_mode = "execute"
plain_text_echo = "snapshot"
plain_text_echo_lines = 12
plain_text_echo_delay = "250ms"
plain_text_echo_timeout = "2s"
follow_lines = 80
follow_min_interval = "700ms"

[daemon.discord]
token = "discord-bot-token"
command_prefix = "tmux:"
```

This example intentionally enables `plain_text_mode = "execute"` so bare text in
DMs sends and presses Enter immediately. If you want raw text without automatic
Enter, change it back to `type`.

```bash
./tmux-connect daemon run
```

Using explicit flags:

```bash
./tmux-connect daemon run \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN" \
  --discord-command-prefix "tmux:" \
  --plain-text-mode execute \
  --plain-text-echo snapshot \
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
- `--plain-text-mode type|execute`
- `--plain-text-echo off|snapshot`
- `--plain-text-echo-lines 12`
- `--plain-text-echo-delay 250ms`
- `--plain-text-echo-timeout 2s`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`

Supported environment variables:

- `TMUXCONN_PLATFORM=discord`
- `TMUXCONN_DISCORD_TOKEN`
- `TMUXCONN_DISCORD_COMMAND_PREFIX`
- `TMUXCONN_DB_PATH`
- `TMUXCONN_PLAIN_TEXT_MODE`
- `TMUXCONN_PLAIN_TEXT_ECHO`
- `TMUXCONN_PLAIN_TEXT_ECHO_LINES`
- `TMUXCONN_PLAIN_TEXT_ECHO_DELAY`
- `TMUXCONN_PLAIN_TEXT_ECHO_TIMEOUT`
- `TMUXCONN_FOLLOW_DEBUG`

## Choosing `allow_chats` Values

`--allow-chat` and `[daemon].allow_chats` are optional. If you do not set them,
any Discord channel or DM that can reach the bot may use it.

- recommended format: `discord:<channel_id>` or `discord:<dm_id>`
- raw IDs without the `discord:` prefix also work, but the prefix avoids
  collisions with other platforms in shared configs
- to copy the ID in Discord, enable Developer Mode in `User Settings ->
  Advanced -> Developer Mode`, then right-click the target server channel or DM
  conversation and choose `Copy Channel ID`
- when allowing multiple chats, repeat `--allow-chat` or add multiple TOML
  entries

## Health Checks

```bash
./tmux-connect daemon doctor \
  --platform discord \
  --discord-token "$TMUXCONN_DISCORD_TOKEN"

./tmux-connect daemon status --db ~/.tmux-connect/tmux-connect.db
```

`daemon doctor` confirms the token is present and reminds you to enable the Message Content intent for prefix commands and DMs.

## Using the Bot

- use slash commands such as `/panes`, `/select`, `/snapshot`, or `/follow`
- in guild channels, you can also use prefixed commands such as `tmux: panes`
- in DMs, plain text always targets the current pane; default `type` mode keeps it raw, while execute mode sends text and Enter in one step
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
- DMs treat plain text as pane input; default `type` mode keeps it raw, while `plain_text_mode = "execute"` makes bare text send and press Enter
- snapshot defaults to image when rendering succeeds; text mode is still available
- current pane bindings and message continuity survive daemon restarts through SQLite
- allowlists should prefer `discord:<channel_id>` or `discord:<dm_id>` entries to avoid collisions with other platforms
