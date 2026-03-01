# agent-swarm

Go CLI for orchestrating parallel coding agents across isolated git worktrees with dependency tracking and phase gates.

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
effort = ""
bypass_sandbox = true

[notifications]
type = "stdout"
telegram_chat_id = ""
telegram_token = ""

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
