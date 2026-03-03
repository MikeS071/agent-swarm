# agent-swarm

Go CLI for orchestrating parallel coding agents across isolated git worktrees with dependency tracking and phase gates.

<img width="1511" height="1064" alt="image" src="https://github.com/user-attachments/assets/10349ec9-1c0c-4295-935f-d570f0b50b43" />

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
swarm add-ticket sw-01 --phase 1 --desc "implement tracker"
swarm add-ticket sw-02 --phase 1 --deps sw-01 --desc "status output"

# 3) Create or generate prompts
swarm prompts check
swarm prompts gen sw-01

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
  Scaffold swarm.toml, swarm/tracker.json, swarm/prompts/, swarm/features/, swarm/logs/, and embedded assets

swarm add-ticket <id> [--deps a,b] [--phase N] [--desc "..."]
  Add ticket metadata to tracker

swarm prompts check
  Report todo tickets missing prompts

swarm prompts gen <ticket>
  Generate prompt template for a ticket

swarm status [--project NAME] [--json] [--compact] [--watch] [--live]
  Show tracker status table/JSON/compact, run Bubble Tea TUI, or 1s live terminal view

swarm watch [--interval 5m] [--once] [--dry-run]
  Run watchdog daemon or a single pass

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

swarm install [--user] [--interval 5m] [--uninstall]
  Install/uninstall scheduled swarm watch execution (systemd/launchd/cron)
```

Global flag: `--config swarm.toml` (path to config file).

## Operational Workflow (v2)

1. `swarm init <project>` to scaffold project + agent assets.
2. Add tickets with `swarm add-ticket` and dependencies/phases.
3. Create prompts manually or with `swarm prompts gen`.
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
tracker = "swarm/tracker.json"
auto_approve = false
spec_file = ""
default_profile = ""

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
