# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

tmux-connect is a Go-based local tmux bridge for controlling existing panes from a CLI, HTTP API, or multi-platform relay daemon. It attaches to existing tmux panes only — it does not create them. Tmux is the source of truth for pane existence and managed metadata.

## Build and Test Commands

No Makefile — use the Go toolchain directly:

```bash
go build ./cmd/tmux-connect      # build the CLI binary
go test ./...                    # full test suite
go test ./internal/daemon/...    # single package tests
go test -run TestRouterFollow ./internal/daemon/  # single test
```

Format with `gofmt`. There is no separate lint command configured.

## Architecture

Three operating modes share a common service layer, all routed from `cmd/tmux-connect/main.go`:

```
CLI commands  ──→ tmuxconn.App ──→ tmuxconn.Service ──→ tmux.Client
HTTP server   ──→ httpapi.Server ──→ tmuxconn.Service ──→ tmux.Client
Remote daemon ──→ daemon.Router ──→ tmuxconn.Service ──→ tmux.Client
```

**`internal/tmuxconn/`** — Service layer. `Service` wraps tmux operations (list/attach/detach/inspect/snapshot/send/stream). `App` handles CLI argument parsing and tabular/JSON output.

**`internal/tmux/`** — Tmux CLI wrapper. `Client` calls tmux via `RealRunner` (exec-based). `Target` is a socket+PaneID tuple. Bridge state is stored as tmux user options (`@tmuxconn_managed`, `@tmuxconn_mode`, `@tmuxconn_agent`, `@tmuxconn_label`, etc.) so it survives CLI restarts. Streaming prefers control mode, falls back to polling.

**`internal/httpapi/`** — RESTful HTTP server. Endpoints under `/v1/panes/*` plus `/healthz`. Streaming is SSE-based.

**`internal/daemon/`** — Multi-connector relay daemon. `Router` dispatches bot commands. `FollowManager` streams pane output to chats. `Store` is SQLite-backed persistence (shells out to `sqlite3` CLI, not an embedded driver). `ReplyBus`/`Messenger` handle reply continuity across Telegram, Slack, and Discord. Schema is versioned via `PRAGMA user_version`.

**`internal/telegram/`** — Thin long-polling Telegram Bot API client.

**`internal/slack/`** — Slack Socket Mode and Web API client wrappers.

**`internal/discord/`** — Discord gateway and interaction client wrappers.

## Key Conventions

- **Imports**: standard Go grouping (stdlib, external, internal)
- **Naming**: `PascalCase` exported, `camelCase` internal; test names read as behavior: `TestRouterFollow`
- **Testing**: table-driven with lightweight fakes beside the code they test; use `t.Parallel()` when isolated, `t.TempDir()` for SQLite/filesystem state
- **Commits**: short imperative subjects with Conventional Commit prefixes (`fix:`, `feat:`)
- **Packages**: prefer small package-local types; avoid cross-package abstractions; don't replace `tmuxconn.Service` with generic interfaces

## Runtime Requirements

- `tmux` installed with an existing target pane
- the daemon additionally needs `sqlite3` in PATH and valid platform tokens such as `TMUXCONN_TELEGRAM_TOKEN`, `TMUXCONN_SLACK_BOT_TOKEN`, or `TMUXCONN_DISCORD_TOKEN`
- `TMUXCONN_TMUX_SOCKET` env var overrides the default tmux socket
- Don't commit the root-level `tmux-connect` binary or `.cache/` directory

## Exit Codes

0 = success, 2 = invalid input, 3 = pane not found, 4 = tmux communication error

## Design Principles

- **Tmux-first**: tmux is authoritative for pane state and metadata
- **Relay-mode only**: no structured agent protocol parsing yet
- **Restart-safe**: daemon restart must not orphan managed panes; recovery comes from tmux user options + SQLite state
- **Pane identity from tmux**: never derive pane identity from Telegram chat/session IDs
