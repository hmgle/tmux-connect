# Phase 4 TODO

## Primary Goal

Build the first optional structured-agent layer on top of the current Telegram relay foundation without breaking plain relay mode.

## High-Priority TODO

- introduce a minimal structured adapter boundary for supported agents
- start with `codex` as the first structured target
- define the smallest event model needed for approval, continue, stop, and assistant-output continuity
- keep parser failure and unsupported panes on the plain relay path
- use `message_links` to support future Telegram inline actions

## Telegram TODO

- add inline button / callback support
- map inline actions onto persisted message links instead of transient in-memory message ids
- decide which replies should be new messages versus edits
- improve group-chat behavior without changing the current chat-shared continuity model by default

## tmux And Relay TODO

- support mixed structured + raw follow output
- normalize repaint-heavy output well enough that adapter parsing is not brittle
- add richer control input beyond `Enter` and `Ctrl-C`
- consider follow restore after restart only if it does not complicate the tmux-first model

## Data Layer TODO

- start writing real `agent_session_id` and `agent_thread_id` values when the first adapter is ready
- extend `message_links` only where inline callbacks or edit targets require it
- replace shelling out to `sqlite3` with an in-process DB layer when it becomes the bottleneck

## Quality TODO

- add tests for structured success and fallback-to-relay behavior
- add tests for Telegram callback handling once inline actions land
- add integration tests covering daemon restart with persisted session/message-link state
- document the next operational workflow once inline actions or structured mode exist
