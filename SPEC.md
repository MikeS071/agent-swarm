# agent-swarm — Go CLI for Multi-Agent Development Orchestration

## Overview
Open-source Go CLI that orchestrates parallel coding agents across isolated git worktrees with dependency graphs, phase gates, and self-healing watchdog. Installable via `go install`, usable standalone or as an OpenClaw/ClawHub skill.

## Repository
`github.com/MikeS071/agent-swarm`

## Commands (Cobra)

```
swarm init <project>                    # Scaffold project: swarm.toml + tracker + prompts/
swarm status [--project X] [--json]     # Dispatcher status (all or one project)
swarm spawn <ticket> [--force]          # Create worktree + launch agent for ticket
swarm done <ticket> [sha]               # Mark complete, trigger dependency chain
swarm fail <ticket>                     # Mark failed, report blocked downstream
swarm watch [--interval 5m]             # Watchdog daemon (long-running)
swarm add-ticket <id> --deps a,b --phase 2 --desc "..."
swarm prompts check                     # Verify all todo tickets have prompt files
swarm prompts gen <ticket>              # Generate prompt template for ticket
swarm cleanup [--older-than 24h]        # Remove finished worktrees
swarm go [project]                      # Approve phase gate, spawn next phase
swarm integrate [--base main]           # Merge all done branches into integration branch
```

## Config — `swarm.toml` (per project root)

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
type = "codex-tmux"           # codex-tmux | claude-code | openclaw-subagent
model = "gpt-5.3-codex"
binary = ""                   # auto-detect if empty
effort = "high"
bypass_sandbox = true

[notifications]
type = "stdout"               # stdout | telegram | discord | openclaw
telegram_chat_id = ""
telegram_token = ""

[watchdog]
interval = "5m"
max_runtime = "45m"
stale_timeout = "10m"
max_retries = 2
```

## Architecture

```
cmd/
├── root.go          # Cobra root
├── init.go
├── status.go
├── spawn.go
├── done.go
├── watch.go
├── serve.go         # HTTP API server
├── install.go
├── prompts.go
├── cleanup.go
└── go_cmd.go        # "swarm go" (phase gate approval)

internal/
├── config/          # swarm.toml loader
├── tracker/         # Tracker JSON CRUD + dependency resolver
├── dispatcher/      # Phase logic, spawnable calculation, signals
├── backend/
│   ├── backend.go   # AgentBackend interface
│   ├── codex.go     # Codex tmux implementation
│   ├── claude.go    # Claude Code implementation
│   └── subagent.go  # OpenClaw sessions_spawn implementation
├── watchdog/        # Health monitor, failure tracking, auto-chain
├── worktree/        # Git worktree create/remove/prune
├── server/          # HTTP API + SSE event bus
│   ├── server.go    # Router, handlers
│   ├── events.go    # EventBus fan-out to SSE clients
│   └── middleware.go # CORS, auth token
├── progress/        # PROGRESS: marker parser + heuristic fallback
├── notify/
│   ├── notifier.go  # Notifier interface
│   ├── stdout.go
│   ├── telegram.go
│   └── openclaw.go
└── sysinfo/         # RAM check, process detection
```

## Interfaces

```go
type AgentBackend interface {
    Spawn(ctx context.Context, cfg SpawnConfig) (AgentHandle, error)
    IsAlive(handle AgentHandle) bool
    HasExited(handle AgentHandle) bool
    GetOutput(handle AgentHandle, lines int) (string, error)
    Kill(handle AgentHandle) error
    Name() string
}

type Notifier interface {
    Alert(ctx context.Context, msg string) error
    Info(ctx context.Context, msg string) error
}

type SpawnConfig struct {
    TicketID    string
    Branch      string
    WorkDir     string
    PromptFile  string
    Model       string
    Effort      string
}

type AgentHandle struct {
    SessionName string
    PID         int
    StartedAt   time.Time
}
```

## Tracker Format (unchanged)

```json
{
  "project": "myproject",
  "tickets": {
    "feat-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/feat-01",
      "desc": "Description"
    }
  }
}
```

## Dispatcher Signals

| Signal | Meaning |
|---|---|
| (none) | Spawnable tickets exist → auto-spawn |
| PHASE_GATE | Phase complete → notify, wait for `swarm go` |
| ALL_DONE | Project complete 🏁 |
| BLOCKED | No spawnable tickets, deps incomplete |

## Watchdog Loop (`swarm watch`)

```
every interval:
  1. List tmux/agent sessions
  2. For each running agent:
     - Exited + has commits? → done → dispatcher chain → spawn next
     - Exited + no commits (1st)? → auto-respawn
     - Exited + no commits (2nd)? → mark failed, notify
     - Alive > max_runtime? → notify "stuck?"
  3. If 0 agents running + spawnable tickets + RAM OK → idle auto-spawn
  4. Phase gate? → notify
```

## Live Dashboard — `swarm status --watch`

Bubbletea TUI that refreshes every 3 seconds:

```
┌─ AgentSquads ─────────────────────────────────────────────┐
│ Progress: ████████████░░░░░░░░░░░░░░░░ 13/27 (48%)       │
│ Phase: 2 │ Agents: 2/7 │ RAM: 4.9 GB free                │
├───────────────────────────────────────────────────────────┤
│ ✅ of-01 OpenFang container          done    (30c2d73)    │
│ ✅ of-02 Config injection            done    (f0b547d)    │
│ 🔄 of-07 Events SSE                 running  [████░░] 4/6│
│ 🔄 of-14 Agent grid                 running  [██░░░░] 2/8│
│ ⏳ of-11 Pricing page               queued  (needs of-05)│
│ 🔒 of-16 Chat integration           blocked (of-04,07,14)│
└───────────────────────────────────────────────────────────┘
  q: quit │ Enter: view agent output │ k: kill agent │ r: respawn
```

### Per-Ticket Progress Tracking

**Primary: Agent progress markers (Option C)**

Prompt template includes:
```
After completing each task, output on its own line: PROGRESS: X/N
```

The CLI scrapes the tmux pane for the last `PROGRESS:` line → maps to progress bar.

**Fallback: File change heuristic (Option B)**

When no `PROGRESS:` marker is found, infer from observable state:

| Signal | Estimated % |
|---|---|
| Agent just started, 0 files changed | 5% |
| Files created, no build yet | 30% |
| `thinking` in output, files changing | 50% |
| Build/test output detected | 70% |
| `git commit` in output | 90% |
| `git push` in output | 95% |
| Commit on remote + agent exited | 100% |

**Implementation:**
```go
type TicketProgress struct {
    TicketID    string
    Status      string  // todo, running, done, failed, blocked
    Progress    int     // 0-100
    Source      string  // "marker" or "heuristic"
    TasksDone   int     // from PROGRESS: X/N
    TasksTotal  int
    LastOutput  string  // last meaningful line from agent
    RunningFor  time.Duration
}

func GetProgress(handle AgentHandle, promptTasks int) TicketProgress {
    output := scrape_tmux_pane(handle)
    // Try Option C first
    if marker := parse_progress_marker(output); marker != nil {
        return TicketProgress{Progress: marker.X * 100 / marker.N, Source: "marker"}
    }
    // Fall back to Option B
    return infer_from_heuristics(output, handle)
}
```

### Dashboard Keybindings

| Key | Action |
|---|---|
| `q` / `Ctrl+C` | Quit |
| `↑` / `↓` | Navigate tickets |
| `Enter` | View selected agent's live output (tail tmux pane) |
| `k` | Kill selected agent |
| `r` | Respawn selected agent |
| `g` | Approve phase gate (`swarm go`) |
| `p` | Switch project |
| `Tab` | Toggle between compact/detailed view |

### Non-interactive mode

```bash
swarm status                    # One-shot, plain text
swarm status --json             # Machine-readable
swarm status --watch            # Live TUI dashboard
swarm status --watch --compact  # Minimal (one line per ticket)
```

## HTTP API — `swarm serve`

Turns the CLI into a lightweight HTTP API for web UIs (Mission Control, AgentSquads, any dashboard).

```bash
swarm serve [--port 8090] [--cors "*"] [--auth-token ""]
```

### Endpoints

**Status & Projects**
```
GET  /api/projects                    → [{name, progress, phase, agents_running}]
GET  /api/projects/:name/status       → full dispatcher status (tickets, phases, signals)
GET  /api/projects/:name/tickets      → [{id, status, progress, desc, deps, phase, sha, runtime}]
GET  /api/projects/:name/stats        → {done, running, todo, failed, blocked, total, eta_minutes}
```

**Ticket Operations**
```
GET  /api/projects/:name/tickets/:id          → ticket detail + progress
GET  /api/projects/:name/tickets/:id/output   → SSE stream (live tmux pane tail)
POST /api/projects/:name/tickets/:id/kill     → kill agent
POST /api/projects/:name/tickets/:id/respawn  → respawn agent
POST /api/projects/:name/tickets/:id/done     → manual mark done {sha}
POST /api/projects/:name/tickets/:id/fail     → manual mark failed
```

**Phase Gates**
```
GET  /api/projects/:name/phase-gate           → current gate status
POST /api/projects/:name/phase-gate/approve   → approve, spawn next phase
```

**Watchdog**
```
GET  /api/watchdog/status       → {running, last_run, next_run, alerts_pending}
GET  /api/watchdog/log          → last N log lines [?lines=50]
POST /api/watchdog/run          → trigger watchdog pass now
```

**System**
```
GET  /api/health                → {ok, ram_mb, agents_running, uptime}
GET  /api/events                → SSE stream of all events (completions, spawns, failures, gates)
```

### Event Stream (`/api/events`)

Server-Sent Events for real-time UI updates:

```
event: ticket_done
data: {"project":"agentsquads","ticket":"of-07","sha":"abc123","next_spawnable":["of-16","of-18"]}

event: ticket_spawned
data: {"project":"agentsquads","ticket":"of-16","agent":"codex-of-16"}

event: progress
data: {"project":"agentsquads","ticket":"of-14","progress":62,"tasks_done":5,"tasks_total":8}

event: phase_gate
data: {"project":"agentsquads","phase":2,"message":"Phase 2 complete. 6 tickets ready in phase 3."}

event: failure
data: {"project":"agentsquads","ticket":"of-11","attempt":2,"blocked":["of-13"]}

event: ram_warning
data: {"available_mb":890,"threshold_mb":1024}
```

### Auth

Optional bearer token for production use:
```toml
[serve]
port = 8090
cors = ["https://archonhq.ai", "https://agentsquads.ai"]
auth_token = ""  # empty = no auth, set for production
```

### Architecture

```go
// internal/server/server.go
type Server struct {
    dispatcher *dispatcher.Dispatcher
    watchdog   *watchdog.Watchdog
    backends   map[string]backend.AgentBackend
    events     *EventBus  // fan-out to SSE clients
}
```

The server reuses the exact same internal packages as the CLI — no duplication. `swarm serve` just wraps them in HTTP handlers.

### MC / AgentSquads Integration

Both platforms consume the same API:

```typescript
// MC: src/app/dashboard/swarm/page.tsx
// AgentSquads: src/app/admin/swarm/page.tsx

const eventSource = new EventSource('http://localhost:8090/api/events');
eventSource.addEventListener('ticket_done', (e) => {
  const data = JSON.parse(e.data);
  // Update ticket in state, trigger re-render
});
eventSource.addEventListener('progress', (e) => {
  // Update progress bar
});
```

The `swarm serve` process runs alongside the web apps on the same host. MC and AgentSquads reverse-proxy to it or call directly (same-host, no CORS issues with localhost).

## Integration Phase — `swarm integrate`

Parallel branches always need merging before the final audit/test ticket. This command automates the merge pass:

```bash
swarm integrate [--base main] [--branch integration/v1]
```

**What it does:**
1. Creates `integration/v1` from `--base` (default: main)
2. Merges each `done` ticket's branch in dependency order (leaves first, dependents last)
3. On conflict: stops, reports which branch conflicted, opens the worktree for manual resolution
4. After all merges: runs build verification (`swarm.toml` → `[integration].verify_cmd`)
5. Pushes integration branch
6. Updates audit ticket's worktree to point at integration branch
7. Spawns the audit ticket

```toml
[integration]
verify_cmd = "npm run build"        # or "go build ./..."
audit_ticket = "of-22"              # auto-spawn after successful integration
```

This is the bridge between "all tickets done" and "final audit can run." Without it, the audit ticket ghost-loops because main doesn't have the features.

## ClawHub Skill Layer

Thin SKILL.md wrapper:

```markdown
# Agent Swarm
Orchestrate parallel coding agents with dependency graphs and phase gates.

## Prerequisites
- `swarm` binary in PATH (`go install github.com/MikeS071/agent-swarm@latest`)

## Usage
- `swarm init <project>` to scaffold
- `swarm watch` to start watchdog
- `swarm status` to check progress
```

## Installer — `swarm install`

Sets up the system integration so `swarm watch` runs automatically. Detects the init system and installs accordingly.

```
swarm install [--user] [--interval 5m] [--uninstall]
```

**What it does:**

1. **Detect platform:** systemd (Linux), launchd (macOS), or fallback to cron
2. **systemd (Linux):**
   - Writes `~/.config/systemd/user/swarm-watchdog.service` + `.timer`
   - `systemctl --user enable --now swarm-watchdog.timer`
   - Service runs `swarm watch --once` (single pass, timer handles scheduling)
   - Or `swarm watch` as a persistent service (user chooses)
3. **launchd (macOS):**
   - Writes `~/Library/LaunchAgents/com.agentswarm.watchdog.plist`
   - `launchctl load` the plist
   - Runs `swarm watch --once` on interval
4. **Cron (fallback):**
   - Adds `*/5 * * * * /usr/local/bin/swarm watch --once --project all` to user crontab
   - Idempotent — won't duplicate if already present
5. **Verify:** After install, runs a health check: `swarm watch --dry-run`
6. **`--uninstall`:** Removes the timer/plist/cron entry cleanly

**Config stored in `swarm.toml`:**
```toml
[install]
method = "systemd"     # auto-detected, can override
interval = "5m"
run_mode = "timer"     # "timer" (periodic oneshot) or "daemon" (persistent)
```

**`swarm install` output:**
```
✓ Detected: systemd (Linux)
✓ Wrote ~/.config/systemd/user/swarm-watchdog.service
✓ Wrote ~/.config/systemd/user/swarm-watchdog.timer (every 5m)
✓ Enabled and started swarm-watchdog.timer
✓ Dry-run passed — watchdog sees 2 projects, 14 spawnable tickets

Watchdog is live. Check status: swarm watch --status
Uninstall: swarm install --uninstall
```

## Estimate
- **8-10 hours** — port existing Python logic to Go + abstractions + installer + bubbletea TUI + HTTP API/SSE
- Logic is proven across 3 projects (62 tickets completed)
- v1 ships with: codex-tmux backend, stdout + telegram notifications
- v1.1: claude-code backend, openclaw notifications, ClawHub publish

## Dependencies
- github.com/spf13/cobra (CLI framework)
- github.com/pelletier/go-toml/v2 (config)
- github.com/charmbracelet/bubbletea (TUI dashboard)
- github.com/charmbracelet/lipgloss (TUI styling)
- github.com/fatih/color (non-TUI terminal output)
- Standard library for everything else

## Blocked By
- OF-22 (AgentSquads pipeline complete)
