# Roadmap

## Current Baseline

The current repository ships:

- local tmux bridge via CLI and HTTP API
- remote relay daemon for Telegram, Slack, Discord, and WhatsApp
- SQLite-backed chat bindings, sessions, and message links
- reply continuity for pane-scoped remote interactions
- `/keys` command for sending arbitrary tmux key names from all platforms

## Near-Term Priorities

- add a minimal structured adapter boundary while keeping plain relay mode as the fallback
- start with `codex` as the first structured target
- support Telegram inline actions for continue, stop, and approval-style interactions
- improve follow output handling for repaint-heavy terminal streams
- add richer control input beyond the current `/keys` surface

## Data And Runtime Follow-Up

- start storing real `agent_session_id` and `agent_thread_id` values once the first adapter exists
- extend `message_links` only where inline callbacks or edit targets need it
- replace shelling out to `sqlite3` with an embedded driver when it becomes worthwhile
- evaluate follow restore after restart without breaking the tmux-first model

## Quality Work

- add tests for structured parsing success and relay fallback
- add tests for Telegram callback handling once inline actions land
- add integration coverage for daemon restart with persisted reply state

## Longer-Term Ideas

- optionally add bridge-managed pane creation for users who want managed startup flows
- WhatsApp group chat support
- explore richer platform-specific features (Discord threads, Slack canvas, etc.)
