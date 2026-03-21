# TODOS

## Daemon

### Per-chat input profiles for plain-text execute mode

**What:** Add per-chat or app-level input profiles so plain-text behavior can vary between `type` and `execute` instead of using one daemon-wide default.

**Why:** A single daemon may need to support both aggressive one-message command execution and conservative raw-typing workflows without forcing every allowed chat into the same mode.

**Context:** The phase-one plan for the `optimize` branch intentionally keeps plain-text execute behavior daemon-global to minimize diff size and reduce rollout risk. During design and engineering review, the user explicitly asked for either app-level or server-side configurability. The first implementation will ship the server-side default only, with router-local branching and explicit mode enums so this can later grow into per-chat or app-level overrides without reworking tmux send/snapshot primitives. When revisiting this, start from `internal/daemon/router.go`, `internal/daemon/store.go`, and the daemon config surface, and decide whether the override belongs in persistent chat state, platform-specific app settings, or both.

**Effort:** M
**Priority:** P2
**Depends on:** Phase-one daemon-global execute mode shipping first
