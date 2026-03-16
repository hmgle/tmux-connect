# Telegram Setup and Operations

## Prerequisites

- `tmux` is installed and your target pane already exists
- `sqlite3` is installed and available in `PATH`
- you have created a Telegram bot through BotFather
- you know the Telegram chat ID you want to allow, if using `--allow-chat`

## Local Preparation

1. Attach the pane you want to control:

```bash
go run ./cmd/tagb attach --pane %5 --agent codex --label backend
```

2. Verify that the pane is managed:

```bash
go run ./cmd/tagb inspect --pane %5
```

## Start the Daemon

Using an env var:

```bash
export TAGB_TELEGRAM_TOKEN=123456:example-token
export TAGB_TELEGRAM_SNAPSHOT_THEME=light
export TAGB_TELEGRAM_SNAPSHOT_FONT_SIZE=16
go run ./cmd/tagb daemon run --db ~/.tagb/tagb.db --allow-chat 123456789
```

Using a flag:

```bash
go run ./cmd/tagb daemon run \
  --telegram-token 123456:example-token \
  --db ~/.tagb/tagb.db \
  --telegram-snapshot-theme light \
  --telegram-snapshot-font-size 16 \
  --telegram-snapshot-font-file /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf \
  --allow-chat 123456789
```

## Health Checks

Validate the runtime prerequisites:

```bash
go run ./cmd/tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"
```

Inspect persisted state and current managed pane count:

```bash
go run ./cmd/tagb daemon status --db ~/.tagb/tagb.db
```

## Telegram Commands

- `/panes`
- `/attach <pane>`
- `/detach <pane>`
- `/bind <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/enter`
- `/ctrlc`
- `/follow on|off`

## Typical Flow

1. Start the daemon
2. Open the bot in Telegram
3. Run `/panes`
4. Bind the chat to a managed pane with `/bind %5`
5. Inspect the current pane with `/current`
6. Read output with `/snapshot`
7. Continue the session with `/send continue`
8. Use `/follow on` when you want pushed output updates

## Operational Notes

- If `--allow-chat` is used, chats not in the allowlist are rejected
- `/bind` only works on managed panes; use `/attach` first if needed
- `/snapshot` defaults to `image`; use `/snapshot text` to receive Telegram text instead of an image
- snapshot images default to the built-in `gomono` font, `14` pt, and the `dark` theme
- use `--telegram-snapshot-theme`, `--telegram-snapshot-font-size`, or `--telegram-snapshot-font-file` to customize snapshot image rendering
- `/detach` clears chat bindings and stops any follow sessions that point to that pane
- if the current pane disappears, the daemon clears that chat's current-pane state and asks the user to bind again
- follow mode is one active subscription per chat
