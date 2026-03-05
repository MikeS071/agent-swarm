# agent-swarm

Go CLI for orchestrating parallel coding agents across isolated git worktrees with dependency tracking and phase gates.

<img width="1508" height="606" alt="image" src="https://github.com/user-attachments/assets/430ff7ab-efc3-42f7-8b82-a24d17ab1f90" />


## Install

```bash
go install github.com/MikeS071/agent-swarm@latest
```

## v2 Highlights

- Scaffold includes governance + agent assets: `AGENTS.md`, `.agents/skills/`, `.agents/profiles/`, `.codex/rules/`, and `swarm/features/`.
- Phase-sequential dispatcher with explicit phase-gate approval (`swarm go` or TUI `A`) and optional `auto_approve` mode.
- Layered prompt assembly at spawn time: `AGENTS.md` -> `spec_file` -> profile -> ticket prompt -> `swarm/prompt-footer.md`.
- Operational tools for integration, archiving, live TUI monitoring, HTTP API, and scheduler installation.

## Quick Start

```bash
# 1) Scaffold a project
swarm init myproject
cd myproject

# 2) Add tickets
swarm add-ticket sw-01 --phase 1 --role code-agent --verify-cmd "go test ./..." --desc "implement tracker"
swarm add-ticket sw-02 --phase 1 --deps sw-01 --role code-agent --verify-cmd "go test ./..." --desc "status output"

# 3) Create or generate prompts
swarm prompts check

# 4) Monitor status
swarm status
swarm status --json
swarm status --watch

# 5) Run watchdog loop (or one pass)
swarm watch
# or
swarm watch --once
```

## Commands

```text
swarm init <project>
  Scaffold project + run post-init compliance checks (prep/smoke/install watchdog)

swarm add-ticket <id> [--deps a,b] [--phase N] [--desc "..."]
  Add ticket metadata to tracker

swarm prompts check
  Report todo tickets missing prompts

swarm status [--project NAME] [--json] [--compact] [--watch] [--live]
  Show tracker status table/JSON/compact, run Bubble Tea TUI, or 1s live terminal view

swarm prep [--json]
  Run strict preflight gate (required before watch)

swarm doctor [--json]
  Show hard-gate readiness summary

swarm plan optimize [--only-todo] [--json] [--apply]
  Compute/apply throughput-oriented ticket priorities

swarm watch [--interval 5m] [--once] [--dry-run]
  Run watchdog daemon or a single pass

swarm notify reset-completion
  Clear completion marker so the next ALL_DONE can notify once again

swarm go
  Approve the current phase gate

swarm integrate [--base main] [--branch integration/v1] [--dry-run] [--continue]
  Merge done ticket branches in dependency order onto integration branch

swarm archive [--phase N] [--dry-run] [--restore]
  Archive done tickets to swarm/archive.json or restore archived tickets

swarm cleanup [--older-than 24h]
  Remove stale worktrees

swarm serve [--port 8090] [--cors ORIGIN] [--auth-token TOKEN]
  Run HTTP API + SSE server

swarm install [--interval 5m] [--uninstall]
  Install/uninstall scheduled multi-project watchdog (systemd/launchd/cron)

swarm watchdog run-all-once [--dry-run] [--json]
  Execute one watchdog pass for every registered project
```

Global flag: `--config swarm.toml` (path to config file).

## Operational Workflow (v2)

1. `swarm init <project>` to scaffold project + agent assets and verify prerequisites.
2. Add tickets with `swarm add-ticket` and dependencies/phases.
3. Create prompts for todo tickets (or use your ticket-prep pipeline) and validate with `swarm prep`.
4. Start orchestration with `swarm watch`.
5. Review phase completion via `swarm status` / `swarm status --watch`.
6. Approve gates with `swarm go` (CLI) or `A` (TUI), unless `project.auto_approve = true`.
7. Merge completed work with `swarm integrate`.
8. Archive completed tickets with `swarm archive`.

## Configuration (`swarm.toml`)

`swarm.toml` lives at the project root.

```toml
[project]
name = "myproject"
repo = "."
base_branch = "main"
max_agents = 7
min_ram_mb = 1024
prompt_dir = "swarm/prompts"
state_dir = ".local/state"
tracker = "swarm/tracker.json"
features_dir = "swarm/features"
auto_approve = false
spec_file = ""
require_explicit_role = true
require_verify_cmd = true

[backend]
type = "codex-tmux"
model = "gpt-5.3-codex"
binary = ""
effort = "high"
bypass_sandbox = true

[notifications]
type = "stdout"
telegram_chat_id = ""
telegram_token = ""
telegram_token_cmd = ""

[watchdog]
interval = "5m"
max_runtime = "45m"
stale_timeout = "10m"
max_retries = 2

[post_build]
order = ["doc"]
parallel_groups = []
require_integrated_base = true
integrated_base_branch = "dev"

[guardian]
enabled = true

[integration]
verify_cmd = ""
audit_ticket = ""

[serve]
port = 8090
cors = []
auth_token = ""

[install]
method = ""
interval = "5m"
run_mode = "timer"
```

## TUI (`swarm status --watch`)

Interactive multi-project dashboard with live ticket controls.

| Key | Action |
|-----|--------|
| `q` | Quit |
| `Enter` | View selected ticket output |
| `Esc` | Back to list |
| `↑/↓` | Navigate tickets |
| `k` | Kill selected agent |
| `r` | Respawn selected ticket |
| `A` | Approve phase gate |
| `m` | Toggle auto/manual mode (persists to `swarm.toml`) |
| `p` | Cycle projects |
| `[` / `]` | Previous / next page |
| `Tab` | Toggle compact mode |


## Deterministic Completion Protocol

Swarm does **not** trust Codex exit code alone. Ticket completion is determined by:

1. Runtime exit artifact (`<state_dir>/runs/<ticket>/exit.json`) or backend exit detection
2. Meaningful git diff (ignores prompt/runtime noise files)
3. Verify gate success (`ticket.verify_cmd` or integration fallback)
4. Guardian before-mark-done decision

`swarm status` performs reconciliation in non-watch mode so dead sessions converge quickly.

## Architecture

```text
CLI (cmd/*)
  |
  +-- config loader (internal/config)
  +-- tracker state + archive (internal/tracker)
  +-- dispatcher phase logic (internal/dispatcher)
  +-- watchdog orchestration + event log (internal/watchdog)
  +-- backend adapter (internal/backend)
  +-- worktree manager (internal/worktree)
  +-- notifier adapters (internal/notify)
  +-- status TUI (internal/tui)
  +-- HTTP API + SSE (internal/server)
```

For lifecycle-oriented v2 design notes and planned `feature` command set, see `docs/AGENT-SWARM-V2-SPEC.md`.

## Development

```bash
go test ./... -count=1
go build ./...
go vet ./...
```

## Runtime refresh (after every patch/rebuild)

Use the deterministic runtime refresh script to keep CLI binary + systemd watchdog service aligned:

```bash
scripts/refresh-runtime.sh
# verify only
scripts/refresh-runtime.sh --check-only
```

The script enforces canonical runtime path (`~/.local/bin/agent-swarm`), reloads systemd user units, and validates binary/service hash parity.

## Multi-project watchdog (recommended in OpenClaw)

For OpenClaw environments running multiple swarm projects, run a single timer that calls `swarm watch --once` per project config.

- dedupe by `swarm.toml` realpath (avoid duplicate runs for aliases)
- keep each pass short (`--once`)
- rely on completion marker (`swarm/.completion-notified`) to avoid duplicate ALL_DONE alerts

If needed, reset completion notifications manually:

```bash
swarm notify reset-completion
```
