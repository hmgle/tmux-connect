# Repository Guidelines

## Project Structure & Module Organization
`cmd/tagb` contains the CLI entrypoint for the local bridge, HTTP server, and Telegram daemon. Core logic lives under `internal/`: `tagb` exposes the service/app layer, `tmux` wraps tmux operations and stream handling, `httpapi` serves the local control plane, `daemon` manages Telegram routing and SQLite-backed state, and `telegram` contains the Bot API client. Keep new code inside the closest existing package, and place design or handoff notes in `docs/`.

## Build, Test, and Development Commands
Use the Go toolchain directly; there is no `Makefile`.

- `go build ./cmd/tagb` builds the CLI binary.
- `go run ./cmd/tagb list` exercises the local CLI against the current tmux server.
- `go run ./cmd/tagb serve --listen 127.0.0.1:8080` starts the HTTP API.
- `go run ./cmd/tagb daemon doctor --telegram-token "$TAGB_TELEGRAM_TOKEN"` checks daemon prerequisites.
- `go test ./...` runs the full test suite across all packages.

## Coding Style & Naming Conventions
Format all Go code with `gofmt`; keep imports grouped in the standard Go style. Follow Go naming rules: exported identifiers use `PascalCase`, internal helpers use `camelCase`, and tests should read like behavior statements, for example `TestRouterFollow`. Prefer small package-local types over cross-package abstractions, and keep tmux-first behavior explicit instead of hiding it behind generic interfaces.

## Testing Guidelines
Keep tests beside the code they cover in `*_test.go` files. Favor table-driven tests and lightweight fakes, as used in `internal/tmux` and `internal/daemon`. Use `t.Parallel()` when the test is isolated, and use `t.TempDir()` for SQLite or filesystem state. Any change to command routing, tmux metadata, HTTP handlers, or Telegram follow behavior should include or update package tests.

## Commit & Pull Request Guidelines
Recent history follows short imperative subjects, usually with Conventional Commit prefixes such as `fix:` and `feat:`. Prefer messages like `fix: preserve follow context for inline updates` over generic summaries. Pull requests should include the behavior change, the packages touched, and the exact verification command(s) run. For CLI, HTTP, or Telegram workflow changes, add a short example request, response, or transcript instead of screenshots.

## Runtime & Configuration Tips
Local development assumes `tmux` is installed and that the target pane already exists. The Telegram daemon also requires `sqlite3` in `PATH` and a valid `TAGB_TELEGRAM_TOKEN`. Avoid committing local build artifacts such as the root-level `tagb` binary or cache directories.
