---
name: agent-swarm
description: "Orchestrate parallel coding agents with dependency graphs, phase gates, and auto-chaining. Use when: (1) building multi-ticket features with parallel agents, (2) managing agent swarms across worktrees, (3) tracking ticket progress with TUI dashboard, (4) integrating parallel branches in dependency order. NOT for: single-file edits, quick fixes, or tasks that don't need parallelism."
homepage: https://github.com/MikeS071/agent-swarm
metadata:
  {
    "openclaw":
      {
        "emoji": "🐝",
        "requires": { "bins": ["agent-swarm"], "anyBins": ["codex", "claude"] },
        "install":
          [
            {
              "id": "go",
              "kind": "go",
              "module": "github.com/MikeS071/agent-swarm@latest",
              "bins": ["agent-swarm"],
              "label": "Install agent-swarm (go install)",
            },
            {
              "id": "script",
              "kind": "shell",
              "command": "curl -sSL https://raw.githubusercontent.com/MikeS071/agent-swarm/main/install.sh | bash",
              "bins": ["agent-swarm"],
              "label": "Install agent-swarm (script)",
            },
          ],
      },
  }
---

# Agent Swarm

Orchestrate parallel coding agents across isolated git worktrees with dependency graphs, phase gates, and automatic chaining.

## Prerequisites
- `agent-swarm` binary in PATH (`go install github.com/MikeS071/agent-swarm@latest`)
- A coding agent backend: `codex` (default) or `claude`
- `tmux` installed
- `git` installed

## Quick Start

### 1. Initialize a project

```bash
cd ~/projects/my-app
agent-swarm init my-app
```

Creates `swarm.toml`, `swarm/tracker.json`, and `swarm/prompts/`.

### 2. Add tickets with dependencies

```bash
agent-swarm add-ticket feat-01 --phase 1 --desc "Database schema"
agent-swarm add-ticket feat-02 --phase 1 --desc "Auth system"
agent-swarm add-ticket feat-03 --phase 2 --deps feat-01,feat-02 --desc "API endpoints"
agent-swarm add-ticket feat-04 --phase 3 --deps feat-03 --desc "Final audit + tests"
```

### 3. Write prompts

```bash
agent-swarm prompts check          # show missing prompts
agent-swarm prompts gen feat-01    # generate template
```

Each ticket needs `swarm/prompts/<ticket-id>.md` — the agent's task brief.

### 4. Start the swarm

```bash
agent-swarm watch                  # daemon mode (checks every 5m)
agent-swarm watch --once           # single pass
agent-swarm watch --dry-run        # preview only
```

### 5. Monitor

```bash
agent-swarm status                 # table view
agent-swarm status --json          # machine-readable
agent-swarm status --watch         # live TUI with progress bars
```

### 6. TUI keybindings

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate tickets |
| `Enter` | View agent output |
| `Esc` | Back to list |
| `k` | Kill selected agent |
| `r` | Respawn selected ticket |
| `A` | Approve phase gate |
| `m` | Toggle auto/manual mode (persists to swarm.toml) |
| `p` | Cycle through projects |
| `[`/`]` | Previous/next page |
| `Tab` | Toggle compact view |
| `q` | Quit |

Title bar shows `[auto]` or `[manual]`. Changes take effect immediately (no restart needed). Failed tickets always block dependents regardless of mode.

### 7. Phase gates

When a phase completes, the watchdog pauses (in manual mode) and waits:
```bash
agent-swarm go                     # approve → next phase spawns
```

Or press `A` in the TUI. In auto mode (`m` to toggle), phases advance automatically.

### 8. Integrate

After all tickets complete, merge branches in dependency order:
```bash
agent-swarm integrate --dry-run    # preview
agent-swarm integrate              # execute merges
agent-swarm integrate --continue   # resume after conflict fix
```

## Commands Reference

| Command | Description |
|---|---|
| `agent-swarm init <project>` | Scaffold project files |
| `agent-swarm status [--json] [--watch]` | Show tracker status |
| `agent-swarm add-ticket <id> --phase N --desc "..."` | Add ticket |
| `agent-swarm prompts check/gen` | Manage prompt files |
| `agent-swarm watch [--once] [--dry-run]` | Run watchdog |
| `agent-swarm done <ticket> [sha]` | Mark ticket complete |
| `agent-swarm fail <ticket>` | Mark ticket failed |
| `agent-swarm go` | Approve phase gate |
| `agent-swarm serve [--port 8090]` | HTTP API + SSE server |
| `agent-swarm integrate [--base main]` | Merge branches in dep order |
| `agent-swarm install [--uninstall]` | Install system service |
| `agent-swarm notify reset-completion` | Reset ALL_DONE notification marker |
| `agent-swarm archive` | Archive done tickets |
| `agent-swarm archive restore` | Restore archived tickets |
| `agent-swarm archive list` | List archived tickets |
| `agent-swarm cleanup [--older-than 24h]` | Remove stale worktrees |

## Configuration (`swarm.toml`)

```toml
[project]
name = "myproject"
repo = "."
base_branch = "main"
max_agents = 7
min_ram_mb = 1024
auto_approve = false       # toggle at runtime with 'm' in TUI
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
features_dir = "swarm/features"

[backend]
type = "codex-tmux"        # codex-tmux | claude-code (future)
model = "gpt-5.3-codex"
bypass_sandbox = true

[watchdog]
interval = "5m"
max_runtime = "45m"
max_retries = 2

[notifications]
type = "stdout"            # stdout | telegram

[integration]
verify_cmd = "go build ./..."
audit_ticket = ""
```

## Concepts

- **Tickets**: Unit of work with ID, phase, dependencies, status, git branch
- **Phases**: Sequential groups. Same-phase tickets run in parallel. Phase gate between phases.
- **Worktrees**: Each agent gets isolated git checkout — no file conflicts
- **Progress**: Agents output `PROGRESS: X/N` markers; fallback heuristic from file/build/commit detection
- **Signals**: spawn (work available), PHASE_GATE (review needed), ALL_DONE (🏁), BLOCKED (deps stuck)

## HTTP API (for web dashboards)

```bash
agent-swarm serve --port 8090
```

- `GET /api/projects` — list projects
- `GET /api/projects/:name/tickets` — all tickets + progress
- `GET /api/events` — SSE real-time event stream
- `POST /api/projects/:name/phase-gate/approve` — approve gate

## OpenClaw Integration

When used as an OpenClaw skill, the agent can:
1. Call `agent-swarm init` to scaffold a new swarm project
2. Write prompt files for each ticket
3. Start the watchdog via `agent-swarm watch --once` in background
4. Poll `agent-swarm status --json` to report progress
5. Use `agent-swarm go` to approve phase gates
6. Run `agent-swarm integrate` when all tickets complete

The HTTP API (`agent-swarm serve`) enables Mission Control / web dashboard integration via SSE events.


## OpenClaw multi-project mode (recommended)

For OpenClaw, run swarm in `watch --once` mode per project via a central timer/service. This is safer than one long-running process per repo and avoids orphan loops.

Checklist:
- iterate projects registry
- resolve `<repo>/swarm.toml`
- dedupe identical config paths
- execute `agent-swarm --config <path> watch --once`
- rely on completion marker dedupe for one-time ALL_DONE alerts

If you need to re-send completion after a major change:

```bash
agent-swarm --config <path> notify reset-completion
```
