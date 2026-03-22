# Repository Guidelines

## Project Structure & Module Organization
`cmd/tmux-connect` contains the CLI entrypoint for the local bridge, HTTP server, and multi-platform daemon. Core logic lives under `internal/`: `tmux-connect` exposes the service/app layer, `tmux` wraps tmux operations and stream handling, `httpapi` serves the local control plane, `daemon` manages Telegram/Slack/Discord/WhatsApp routing and SQLite-backed state, and the platform packages (`telegram`, `slack`, `discord`, `whatsapp`) contain connector clients. Keep new code inside the closest existing package, and place design or handoff notes in `docs/`.

## Build, Test, and Development Commands
Use the Go toolchain directly; there is no `Makefile`.

- `go build ./cmd/tmux-connect` builds the CLI binary.
- `go run ./cmd/tmux-connect list` exercises the local CLI against the current tmux server.
- `go run ./cmd/tmux-connect serve --listen 127.0.0.1:8080` starts the HTTP API.
- `go run ./cmd/tmux-connect daemon doctor --telegram-token "$TMUXCONN_TELEGRAM_TOKEN"` checks daemon prerequisites.
- `go test ./...` runs the full test suite across all packages.

## Coding Style & Naming Conventions
Format all Go code with `gofmt`; keep imports grouped in the standard Go style. Follow Go naming rules: exported identifiers use `PascalCase`, internal helpers use `camelCase`, and tests should read like behavior statements, for example `TestRouterFollow`. Prefer small package-local types over cross-package abstractions, and keep tmux-first behavior explicit instead of hiding it behind generic interfaces.

## Testing Guidelines
Keep tests beside the code they cover in `*_test.go` files. Favor table-driven tests and lightweight fakes, as used in `internal/tmux` and `internal/daemon`. Use `t.Parallel()` when the test is isolated, and use `t.TempDir()` for SQLite or filesystem state. Any change to command routing, tmux metadata, HTTP handlers, or remote follow behavior should include or update package tests.

## Commit & Pull Request Guidelines
Recent history follows short imperative subjects, usually with Conventional Commit prefixes such as `fix:` and `feat:`. Prefer messages like `fix: preserve follow context for inline updates` over generic summaries. Pull requests should include the behavior change, the packages touched, and the exact verification command(s) run. For CLI, HTTP, or remote connector workflow changes, add a short example request, response, or transcript instead of screenshots.

## Runtime & Configuration Tips
Local development assumes `tmux` is installed and that the target pane already exists. The daemon also requires `sqlite3` in `PATH` plus valid platform credentials or session config: `TMUXCONN_TELEGRAM_TOKEN`, `TMUXCONN_SLACK_BOT_TOKEN`, `TMUXCONN_DISCORD_TOKEN` for tokens, or `TMUXCONN_WHATSAPP_SESSION_DB` for the WhatsApp local session database path. Avoid committing local build artifacts such as the root-level `tmux-connect` binary or cache directories.
