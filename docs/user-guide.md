# Agent Swarm — User Guide

## What is Agent Swarm?

Agent Swarm is a Go CLI that orchestrates parallel coding agents across isolated git worktrees. You define tickets with dependencies and phases, and the swarm handles spawning agents, tracking progress, chaining unblocked work, and enforcing phase gates.

The CLI command is `swarm`.

## Installation

```bash
go install github.com/MikeS071/agent-swarm@latest
```

Or build from source:

```bash
git clone https://github.com/MikeS071/agent-swarm.git
cd agent-swarm
go build -o swarm .
mv swarm ~/.local/bin/
```

## Getting Started

### 1. Initialize a project

```bash
cd ~/projects/my-app
swarm init my-app
```

This creates:
- `swarm.toml`
- `swarm/tracker.json`
- `swarm/prompts/`
- `swarm/features/`
- `swarm/logs/`
- `AGENTS.md`, `.agents/skills/`, `.agents/profiles/`, `.codex/rules/`

### 2. Define tickets

```bash
# Phase 1
swarm add-ticket feat-01 --phase 1 --desc "Database schema and migrations"
swarm add-ticket feat-02 --phase 1 --desc "Authentication with JWT"
swarm add-ticket feat-03 --phase 1 --desc "REST API scaffold"

# Phase 2
swarm add-ticket feat-04 --phase 2 --deps feat-01,feat-02 --desc "User CRUD endpoints"
swarm add-ticket feat-05 --phase 2 --deps feat-01,feat-03 --desc "API middleware + validation"

# Phase 3
swarm add-ticket feat-06 --phase 3 --deps feat-04,feat-05 --desc "Integration tests + docs"
```

### 3. Create prompts

Each ticket should have `swarm/prompts/<ticket-id>.md`.

```bash
swarm prompts check
swarm prompts gen feat-01
```

If a prompt file is missing at spawn time, the watchdog auto-generates a minimal prompt from ticket description and saves it.

### 4. Configure backend + project context

Edit `swarm.toml`:

```toml
[project]
auto_approve = false
spec_file = "SPEC.md"      # optional project spec included in layered prompts
default_profile = "code-agent"  # optional default profile from .agents/profiles

[backend]
type = "codex-tmux"
model = "gpt-5.3-codex"
effort = "high"
bypass_sandbox = true
```

### 5. Start orchestration

```bash
swarm watch
swarm watch --once
swarm watch --dry-run
```

What the watchdog does:
1. Monitors `running` tickets and detects exits.
2. Marks tickets `done` when commits are detected.
3. Respawns tickets that exit without commits up to retry limit.
4. Marks tickets `failed` after retry exhaustion.
5. Spawns newly unblocked tickets within current phase.
6. Emits `PHASE_GATE` when a phase completes (unless auto-approve advances it).

### 6. Monitor and control

```bash
swarm status
swarm status --json
swarm status --compact
swarm status --live
swarm status --watch
```

TUI keys (`swarm status --watch`):

| Key | Action |
|---|---|
| `↑`/`↓` | Navigate tickets |
| `Enter` | View ticket output |
| `Esc` | Back to list |
| `k` | Kill selected agent |
| `r` | Respawn selected agent |
| `A` | Approve phase gate |
| `m` | Toggle auto/manual mode |
| `p` | Cycle projects |
| `[` / `]` | Previous/next page |
| `Tab` | Toggle compact mode |
| `q` | Quit |

### 7. Phase gates

When all tickets in the current phase are `done`, the dispatcher emits `PHASE_GATE`.

Manual flow:

```bash
swarm status
swarm go
```

Auto flow:

```toml
[project]
auto_approve = true
```

`auto_approve` can also be toggled in TUI with `m` and is persisted to `swarm.toml`.

### 8. Integration

After tickets complete, merge done branches in dependency order:

```bash
swarm integrate --dry-run
swarm integrate --base main --branch integration/v1
swarm integrate --continue
```

`--continue` resumes after conflict resolution using saved integration state.

### 9. Archive completed tickets

```bash
# archive done tickets
swarm archive

# archive done tickets from one phase
swarm archive --phase 2

# preview archive operation
swarm archive --dry-run

# restore archived tickets back into tracker
swarm archive --restore
```

Archive storage path: `swarm/archive.json`.

### 10. API server

```bash
swarm serve --port 8090
```

Common endpoints:
- `GET /api/projects`
- `GET /api/projects/{name}/status`
- `GET /api/projects/{name}/tickets`
- `GET /api/projects/{name}/phase-gate`
- `POST /api/projects/{name}/phase-gate/approve`
- `GET /api/events` (SSE)

### 11. Install scheduler

```bash
swarm install
swarm install --interval 3m
swarm install --uninstall
```

Supported install targets: `systemd` (Linux), `launchd` (macOS), `cron` fallback.

## Prompt Layering (v2 Runtime)

When spawning a ticket, prompt content is assembled in this order:
1. `AGENTS.md`
2. `project.spec_file` (if configured)
3. Profile markdown (`ticket.profile` or `project.default_profile`)
4. Ticket prompt file (`swarm/prompts/<ticket>.md`)
5. `swarm/prompt-footer.md` (if present)

This gives each agent governance context, optional project spec, role-specific behavior, task details, and mandatory delivery process.

See [lessons-learned.md](lessons-learned.md) for hard-won operational knowledge covering:
- Prompt engineering patterns (what works, what kills agents)
- Watchdog failure modes and fixes
- Common agent behaviour patterns and recovery workflows
- Scaling configurations
- Governance validation integration

## Concepts

### Tickets
Unit of work with ID, phase, dependencies, status (`todo`, `running`, `done`, `failed`, `blocked`), branch, and optional profile.

### Phases
Strictly sequential. Tickets only spawn within the currently unlocked phase.

### Dispatcher Signals
| Signal | Meaning |
|---|---|
| `(spawn)` | Spawnable tickets exist in current phase |
| `PHASE_GATE` | Current phase complete, waiting for approval |
| `ALL_DONE` | All tickets done |
| `BLOCKED` | Nothing spawnable (deps, failures, or capacity constraints) |

### Worktrees
Each ticket runs in `<repo>-worktrees/<ticket-id>` on its own branch (default `feat/<ticket-id>`).

## Notes on Feature Lifecycle Commands

The repository includes v2 lifecycle design docs in `docs/AGENT-SWARM-V2-SPEC.md` (feature-state machine and specialist profile flows). The currently implemented CLI command set is the one shown in this guide.


### 12. Reset completion notifications

When a project reaches `ALL_DONE`, completion notifications are deduped using `swarm/.completion-notified`.

If you intentionally want a new completion notification on next pass:

```bash
swarm notify reset-completion
```

This only removes the marker for the current project config.


### 13. Multi-project watchdog

For OpenClaw setups with multiple repos, prefer a multi-project timer that runs:

```bash
swarm --config <repo>/swarm.toml watch --once
```

for each registered project on a short interval (e.g. 1m).

Best practice:
- dedupe by resolved `swarm.toml` path
- skip repos without `swarm.toml`
- let per-project completion marker prevent repeated ALL_DONE notifications
