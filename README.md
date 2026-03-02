# agent-swarm

Go CLI for orchestrating parallel coding agents across isolated git worktrees with dependency tracking and phase gates.

<img width="1511" height="1064" alt="image" src="https://github.com/user-attachments/assets/10349ec9-1c0c-4295-935f-d570f0b50b43" />

## Install

```bash
go install github.com/MikeS071/agent-swarm@latest
```

## Quick Start

```bash
# 1) Scaffold a project
swarm init myproject
cd myproject

# 2) Add tickets
swarm add-ticket sw-01 --phase 1 --desc "implement tracker"
swarm add-ticket sw-02 --phase 1 --deps sw-01 --desc "status output"

# 3) Validate prompts and generate missing templates
swarm prompts check
swarm prompts gen sw-01

# 4) Inspect project status
swarm status
swarm status --json

# 5) Run watchdog loop (or one pass)
swarm watch
# or
swarm watch --once
```

## Commands

```text
swarm init <project>
  Scaffold project files: swarm.toml, swarm/tracker.json, swarm/prompts/

swarm status [--project NAME] [--json] [--watch] [--compact]
  Show tracker status as table, JSON, compact list, or live TUI

swarm add-ticket <id> [--deps a,b] --phase N --desc "..."
  Add a ticket to tracker

swarm prompts check
  Report todo tickets missing prompt files

swarm prompts gen <ticket>
  Generate prompt template for a ticket

swarm cleanup [--older-than 24h]
  Remove stale worktrees older than the provided duration

swarm watch [--interval 5m] [--once] [--dry-run]
  Run watchdog daemon or a single watchdog pass

swarm serve [--port 8090] [--cors ORIGIN] [--auth-token TOKEN]
  Run HTTP API server

swarm install [--user] [--interval 5m] [--uninstall]
  Install/uninstall scheduled watchdog execution (systemd/launchd/cron)

swarm integrate [--base main] [--branch integration/v1] [--dry-run] [--continue]
  Merge done ticket branches in dependency order
swarm archive [--phase N] [--dry-run]
  Archive done tickets to swarm/archive.json

swarm archive restore
  Restore all archived tickets back to tracker

swarm archive list
  Show archived tickets
```

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

[backend]
type = "codex-tmux"
model = "gpt-5.3-codex"
binary = ""
effort = "high"
bypass_sandbox = true

[notifications]
type = "stdout"
telegram_chat_id = ""
telegram_token_cmd = "pass show apis/telegram-bot-token"  # shell command for token

[watchdog]
interval = "5m"
max_runtime = "45m"
stale_timeout = "10m"
max_retries = 2
auto_approve = false  # true = skip phase gates, auto-advance

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


## TUI (`status --watch`)

Interactive terminal dashboard showing all discovered projects.

### Keybindings

| Key | Action |
|-----|--------|
| `q` | Quit |
| `Enter` | View agent output for selected ticket |
| `Esc` | Back to list |
| `↑/↓` | Navigate tickets |
| `k` | Kill selected agent |
| `r` | Respawn selected ticket |
| `A` (or `g`) | Approve phase gate |
| `[` / `]` | Previous / next page |
| `p` | Cycle through projects |
| `a` | Archive all done tickets |
| `Tab` | Toggle compact view |

### Multi-project discovery

The TUI auto-discovers projects by scanning for `swarm.toml` files (depth 3, skips worktrees/node_modules/vendor). Projects are deduplicated by name.

## Architecture

```text
CLI (cmd/*)
  |
  +-- config loader (internal/config)
  +-- tracker state (internal/tracker)
  +-- dispatcher phase/dependency logic (internal/dispatcher)
  +-- watchdog orchestration (internal/watchdog)
  +-- backend adapter (internal/backend)
  +-- worktree manager (internal/worktree)
  +-- notifier adapters (internal/notify)
  +-- status TUI (internal/tui)
  +-- HTTP API + SSE (internal/server)
```

## Development

```bash
go test ./... -v -count=1
go vet ./...
go build ./...
```
