# Slack Setup and Operations

## Prerequisites

- `tmux` is installed and the target pane already exists
- `sqlite3` is installed and available in `PATH`
- you have a Slack app with:
  - a bot token (`xoxb-...`)
  - an app-level token (`xapp-...`) with Socket Mode enabled

## Required Slack App Settings

- enable Socket Mode
- add bot scopes:
  - `app_mentions:read`
  - `chat:write`
  - `im:history`
  - `im:read`
- subscribe to bot events:
  - `app_mention`
  - `message.im`

Socket Mode means you do not need a public webhook URL.

## Start the Daemon

```bash
go run ./cmd/tagb daemon run \
  --platform slack \
  --slack-bot-token "$TAGB_SLACK_BOT_TOKEN" \
  --slack-app-token "$TAGB_SLACK_APP_TOKEN" \
  --db ~/.tagb/tagb.db
```

Useful flags:

- `--platform slack`
- `--slack-bot-token TOKEN`
- `--slack-app-token TOKEN`
- `--slack-command-prefix PREFIX` (default: `tmux:`)
- `--db PATH`
- `--snapshot-lines 120`
- `--follow-lines 80`
- `--follow-min-interval 700ms`
- `--follow-debug`

Supported environment variables:

- `TAGB_PLATFORM=slack`
- `TAGB_SLACK_BOT_TOKEN`
- `TAGB_SLACK_APP_TOKEN`
- `TAGB_SLACK_COMMAND_PREFIX`
- `TAGB_DB_PATH`
- `TAGB_FOLLOW_DEBUG`

## Health Checks

```bash
go run ./cmd/tagb daemon doctor \
  --platform slack \
  --slack-bot-token "$TAGB_SLACK_BOT_TOKEN" \
  --slack-app-token "$TAGB_SLACK_APP_TOKEN"

go run ./cmd/tagb daemon status --db ~/.tagb/tagb.db
```

## Using the Bot

- in a DM with the bot, send commands directly
- in a channel, mention the bot once to start a managed thread
- follow-up commands can continue inside that thread without re-mentioning the bot
- in channels, use an app mention as the command prefix, for example `@tagb panes`
- in DMs and managed threads, prefix commands with `tmux:`, for example `tmux: select %5` or `tmux: send status`
- slash-prefixed forms like `/panes` are still accepted by the router, but Slack may intercept them as real Slash Commands before they reach the bot

This bot intentionally does not depend on Slack Slash Commands for the main workflow. Slash invocations are awkward for thread-based control, while mention-based entry and `tmux: ...` commands keep room for future raw-text tmux input.

Supported commands:

- `tmux: start`
- `tmux: help`
- `tmux: panes`
- `tmux: select <pane>`
- `tmux: clear`
- `tmux: unmanage <pane>`
- `tmux: current`
- `tmux: snapshot [lines] [image|text]`
- `tmux: send <text>`
- `tmux: enter`
- `tmux: ctrlc` or `tmux: ctrl-c`
- `tmux: follow on [interval]|off`

If `tmux: select`, `tmux: unmanage`, `tmux: send`, or `tmux: follow` is sent without arguments, the bot prompts in-thread and waits for the next reply in that same Slack thread.

## Operational Notes

- Slack replies are posted in a thread rooted at the triggering message
- `tmux: snapshot` defaults to `image`; `tmux: snapshot text` forces plain text output
- follow mode is one active subscription per Slack conversation tracked by the daemon
- current pane bindings and reply continuity survive daemon restart through SQLite
- follow subscriptions do not survive daemon restart
