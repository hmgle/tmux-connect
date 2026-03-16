# Phase 2 Spec: Telegram Daemon and Command Surface

> Status: implemented baseline on 2026-03-15

## Daemon Commands

### `tagb daemon run`

Starts the Telegram relay daemon.

Flags:

- `--telegram-token TOKEN`
- `--db PATH`
- `--allow-chat CHAT_ID`
- `--poll-timeout 20s`
- `--snapshot-lines 120`
- `--telegram-snapshot-theme dark|light`
- `--telegram-snapshot-font-size 14`
- `--telegram-snapshot-font-file /path/to/font.ttf`
- `--follow-lines 80`
- `--telegram-api-base URL`

Env vars:

- `TAGB_TELEGRAM_TOKEN`
- `TAGB_DB_PATH`
- `TAGB_TELEGRAM_SNAPSHOT_THEME`
- `TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE`
- `TAGB_TELEGRAM_SNAPSHOT_FONT_FILE`
- `TAGB_TELEGRAM_API_BASE`

### `tagb daemon doctor`

Validates:

- Telegram token presence
- `sqlite3` availability and DB openability
- tmux pane listing access

### `tagb daemon status`

Reports:

- DB path
- registered chat count
- binding count
- message log row count
- managed pane count from tmux

## Telegram Command Surface

### `/start` and `/help`

Print the supported command list.

### `/panes`

Lists visible panes and marks:

- `managed` or `unmanaged`
- `bound` for the current chat
- `current` for the current pane

### `/attach <pane>`

Marks an existing pane as managed using the current relay metadata model.

### `/detach <pane>`

Detaches a pane, clears chat bindings that point to it, and stops follow sessions attached to it.

### `/bind <pane>`

Binds the current Telegram chat to a managed pane and sets it as the current pane.

### `/current`

Shows the current pane and basic metadata for the chat.

### `/snapshot [lines] [image|text]`

Returns a recent pane capture for the current pane. Default mode is `image`; pass `text` to force Telegram text output. Image mode defaults to the built-in `gomono` font, `14` pt, and the `dark` theme, and can be customized with the snapshot image flags or env vars above.

### `/send <text>`

Injects literal text into the current pane.

### `/enter`

Sends `Enter` to the current pane.

### `/ctrlc`

Sends `Ctrl-C` to the current pane.

### `/follow on|off`

- `on`: sends an initial capture and then pushes aggregated output updates
- `off`: stops the active follow subscription for the chat

## Persistence

SQLite currently stores:

- `chat_bindings`
- `chat_state`
- `message_log`

tmux metadata continues to store:

- `@tagb_managed`
- `@tagb_mode`
- `@tagb_agent`
- `@tagb_label`
- `@tagb_created_by`
- `@tagb_last_activity_unix`

## Operational Notes

- daemon startup drains pending Telegram updates before entering the main loop
- follow mode is per chat, not per user inside a group
- if the current pane is missing or no longer managed, the daemon clears that chat's current-pane state and returns an actionable error
