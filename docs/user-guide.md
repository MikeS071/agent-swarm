# Agent Swarm — User Guide

## What is Agent Swarm?

Agent Swarm is a Go CLI that orchestrates parallel coding agents across isolated git worktrees. You define tickets with dependencies and phases, and the swarm handles spawning agents, tracking progress, detecting completions, chaining dependent work, and alerting you at phase gates.

Think of it as a CI system for AI coding agents — except instead of running tests, it runs agents that write code.

## Installation

```bash
go install github.com/MikeS071/agent-swarm@latest
```

Or build from source:

```bash
git clone https://github.com/MikeS071/agent-swarm.git
cd agent-swarm
go build -o agent-swarm .
mv agent-swarm ~/.local/bin/
```

## Getting Started

### 1. Initialize a project

```bash
cd ~/projects/my-app
swarm init my-app
```

This creates:
- `swarm.toml` — project configuration
- `swarm/tracker.json` — ticket state (the source of truth)
- `swarm/prompts/` — directory for agent prompt files

### 2. Define your tickets

```bash
# Phase 1 — no dependencies, can run in parallel
swarm add-ticket feat-01 --phase 1 --desc "Database schema and migrations"
swarm add-ticket feat-02 --phase 1 --desc "Authentication with JWT"
swarm add-ticket feat-03 --phase 1 --desc "REST API scaffold"

# Phase 2 — depends on Phase 1 tickets
swarm add-ticket feat-04 --phase 2 --deps feat-01,feat-02 --desc "User CRUD endpoints"
swarm add-ticket feat-05 --phase 2 --deps feat-01,feat-03 --desc "API middleware + validation"

# Phase 3 — final audit
swarm add-ticket feat-06 --phase 3 --deps feat-04,feat-05 --desc "Integration tests + README"
```

### 3. Write prompts

Each ticket needs a prompt file at `swarm/prompts/<ticket-id>.md`. This is what the AI agent receives as its task.

```bash
swarm prompts check       # shows which tickets are missing prompts
swarm prompts gen feat-01 # creates a template prompt
```

A good prompt includes:
- **Context** — what the project is, what exists already
- **Scope** — exactly what this ticket should build (files, functions, interfaces)
- **Tests** — what tests to write first (TDD)
- **Deliverables** — build passes, tests pass, commit message format

### 4. Configure your backend

Edit `swarm.toml`:

```toml
[backend]
type = "codex-tmux"           # currently supported: codex-tmux
model = "gpt-5.3-codex"       # model to use
bypass_sandbox = true          # --dangerously-bypass-approvals-and-sandbox
```

### 5. Start the watchdog

```bash
swarm watch              # long-running daemon (default 5m interval)
swarm watch --once       # single pass (good for cron)
swarm watch --dry-run    # preview without executing
```

The watchdog:
1. Detects when agents exit with commits → marks them done
2. Auto-spawns the next unblocked tickets
3. Respawns agents that exit without commits (once — fails on second attempt)
4. Alerts on stuck agents (exceeding max runtime)
5. Stops at phase gates and notifies you

### 6. Monitor progress

```bash
swarm status             # quick table
swarm status --json      # machine-readable
swarm status --watch     # live TUI dashboard
```

TUI keybindings:
| Key | Action |
|---|---|
| `↑`/`↓` | Navigate tickets |
| `Enter` | View agent's live output |
| `Esc` | Back to list |
| `k` | Kill selected agent |
| `r` | Respawn selected agent |
| `A` | Approve phase gate |
| `m` | Toggle auto/manual mode (persists to swarm.toml) |
| `p` | Switch project |
| `Tab` | Toggle compact/detailed view |
| `[` | Previous page |
| `]` | Next page |
| `q` | Quit |

### 7. Phase gates

When all tickets in a phase complete, the watchdog stops and waits:

```bash
swarm status    # review completed work
swarm go        # approve → next phase auto-spawns
```

#### Auto-approve mode

For fully autonomous operation, set `auto_approve = true` in `swarm.toml`:

```toml
[project]
auto_approve = true
```

With auto-approve enabled, the watchdog automatically advances through phase gates without waiting for `swarm go`.

You can also toggle this at runtime in the TUI by pressing `m`. The title bar shows `[auto]` or `[manual]` to indicate the current mode. Changes are persisted to `swarm.toml` and take effect on the next watchdog pass (within seconds). No restart needed.

In auto mode, the watchdog automatically approves phase gates and spawns the next phase. In manual mode, it stops at each gate until you press `A` or run `agent-swarm go`. Failed tickets always block their dependents regardless of mode.

Phase gate events are still logged to the event trail for auditability.

### 8. Integration

After all tickets complete, merge branches in dependency order:

```bash
swarm integrate --dry-run                        # preview merge plan
swarm integrate --base main --branch integration/v1  # execute
swarm integrate --continue                       # resume after conflict resolution
```

On conflict, the CLI stops and shows exactly which files conflict and how to resolve.

### 9. System service

```bash
swarm install                  # auto-detect platform, install watchdog
swarm install --interval 3m    # custom interval
swarm install --uninstall      # remove cleanly
```

Supports: systemd (Linux), launchd (macOS), cron (fallback).

## Concepts

### Tickets
A unit of work with: ID, phase, dependencies, status (`todo` → `running` → `done`/`failed`), and a git branch.

### Phases
Sequential groups. All tickets within a phase run in parallel. Phase N+1 starts only after Phase N is complete and approved.

### Dispatcher Signals
| Signal | Meaning |
|---|---|
| (spawn) | Spawnable tickets exist → auto-spawn |
| PHASE_GATE | Phase complete → waiting for `swarm go` |
| ALL_DONE | Project complete 🏁 |
| BLOCKED | No spawnable tickets, deps incomplete |

### Worktrees
Each agent gets an isolated git worktree — a separate checkout on its own branch. Agents never conflict with each other. Branches merge via `swarm integrate`.

### Progress Tracking
**Primary:** Agents output `PROGRESS: X/N` lines → parsed into progress bars.
**Fallback:** Heuristic from file changes (30%), build output (70%), git commit (90%), exit with commits (100%).

## HTTP API

```bash
swarm serve --port 8090
```

Key endpoints:
- `GET /api/projects` — list projects
- `GET /api/projects/:name/tickets` — tickets with progress
- `GET /api/events` — SSE real-time event stream
- `POST /api/projects/:name/tickets/:id/kill` — kill agent
- `POST /api/projects/:name/phase-gate/approve` — approve gate

## Tips

- **The `effort` field maps to `--config model_reasoning_effort=<value>`. Set to `"high"` for best results.
- **Start small:** 2-3 tickets in Phase 1 to validate before scaling to 7 agents
- **Prompt quality matters:** Well-scoped prompts with clear deliverables beat vague ones
- **TDD in prompts:** Tell agents to write tests first — makes completion detection reliable
- **Monitor RAM:** Set `min_ram_mb` — watchdog won't spawn below threshold
- **Phase gates are checkpoints:** Review, test, catch issues before building on them
- **One commit per ticket:** Simplifies integration

## Archiving Done Tickets

After a swarm completes, clean up the tracker by archiving done tickets:

```bash
# Archive all done tickets
agent-swarm archive

# Archive only tickets from phase 2
agent-swarm archive --phase 2

# Preview what would be archived
agent-swarm archive --dry-run

# Restore all archived tickets
agent-swarm archive restore

# View archived tickets
agent-swarm archive list
```

In the TUI (`status --watch`), press `a` to archive done tickets for the current project.

Archived tickets are stored in `swarm/archive.json` alongside the tracker.

## Lessons Learned

See [lessons-learned.md](lessons-learned.md) for hard-won operational knowledge covering:
- Prompt engineering patterns (what works, what kills agents)
- Watchdog failure modes and fixes
- Common agent behaviour patterns and recovery workflows
- Scaling configurations
- Decapod governance integration

## Standard Phase Flow

Unless `auto_approve = true` overrides it, every phase follows this mandatory sequence:

```
Feature tickets (parallel) → int-N (integration merge) → tst-N (E2E test) → Phase gate → Human verifies → Fix → Approve → Next phase
```

Add integration and test tickets to every phase:

```bash
# For each phase N, add:
swarm add-ticket int-N --phase N --deps <all-phase-N-tickets> --desc "Phase N integration merge"
swarm add-ticket tst-N --phase N --deps int-N --desc "Phase N E2E test suite"
```

The integration ticket merges all feature branches, resolves conflicts, and verifies the build. The test ticket runs the full test suite with coverage. Only after both pass does the phase gate fire for human approval.
```

git add -A
git commit -m "docs: standard phase flow — feature → integrate → test → gate → verify → approve"
git push origin main 2>&1 | tail -2
