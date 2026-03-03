# Agent Swarm — Technical Documentation

## Architecture

```text
CLI (cmd/*)
  |
  +-- config (internal/config)
  +-- tracker + archive (internal/tracker)
  +-- dispatcher (internal/dispatcher)
  +-- watchdog (internal/watchdog)
  +-- backend adapters (internal/backend)
  +-- worktree manager (internal/worktree)
  +-- notifier adapters (internal/notify)
  +-- status TUI (internal/tui)
  +-- HTTP API + SSE (internal/server)
  +-- progress parsing (internal/progress)
  +-- RAM checks (internal/sysinfo)
```

## Core Runtime Flow

1. `watchdog.RunOnce` loads current tracker/config context.
2. Running tickets are checked for exited/alive state via backend sessions.
3. Exited tickets are classified as `done` (commit found) or retried/failed.
4. Dispatcher evaluates signal (`spawn`, `PHASE_GATE`, `ALL_DONE`, `BLOCKED`).
5. Spawnable tickets are launched up to capacity constraints.
6. Tracker and `events.jsonl` are updated.

## v2 Prompt Assembly

At spawn time, each ticket prompt is assembled by `watchdog.assemblePrompt`:

1. `AGENTS.md` (project governance)
2. `project.spec_file` (optional)
3. profile markdown (`ticket.profile` or `project.default_profile`)
4. ticket prompt file (`swarm/prompts/<ticket>.md`)
5. `swarm/prompt-footer.md` (optional)

Output path in worktree: `.codex-prompt.md`.

## Package Reference

### `internal/config`

- Loads `swarm.toml` and applies defaults.
- Validates required fields.
- Resolves Telegram token from command if configured.

Notable project fields:
- `auto_approve`
- `spec_file`
- `default_profile`

### `internal/tracker`

Tracker JSON is the state source of truth.

`Ticket` fields include:
- `status`, `phase`, `depends`, `branch`, `desc`
- `profile` (optional)
- `sha`, `startedAt`, `finishedAt`

Archive support:
- `ArchiveDoneTickets(...)`
- `RestoreArchivedTickets(...)`
- default archive path: `swarm/archive.json`

### `internal/dispatcher`

Signal model:
- `SignalSpawn` (`""`)
- `SignalPhaseGate`
- `SignalAllDone`
- `SignalBlocked`

Behavior:
- Strict phase-sequential spawning.
- Optional auto-approval via `project.auto_approve`.
- Capacity checks include `max_agents` and RAM threshold.

### `internal/watchdog`

Responsibilities:
- Spawn tickets via worktree + backend.
- Retry failed exits without commits.
- Mark tickets failed after retry limit.
- Detect long-running tickets via `max_runtime`.
- Persist JSONL events to `swarm/events.jsonl`.

Watchdog event types emitted to JSONL:
- `ticket_done`
- `respawn`
- `ticket_failed`
- `idle_spawn`
- `phase_gate_auto_approved`
- `phase_gate`
- `project_complete`
- `ticket_spawned`

### `internal/backend`

`AgentBackend` abstraction with current implementation `codex-tmux`.

`CodexBackend`:
- launches tmux sessions named `swarm-<project>_<ticket>`
- runs `codex exec` with configured model/effort/sandbox flags
- captures output via `tmux capture-pane`

### `internal/worktree`

Manages git worktree lifecycle under `<repo>-worktrees/`.

Main operations:
- `Create(ticketID, branch)`
- `Remove(ticketID)`
- `CleanupOlderThan(duration)`
- `HasCommits(ticketID, baseBranch)`

### `internal/server`

Provides HTTP API and SSE streams.

Routes:
- `GET /api/projects`
- `GET /api/projects/{name}/status`
- `GET /api/projects/{name}/tickets`
- `GET /api/projects/{name}/stats`
- `GET /api/projects/{name}/tickets/{id}`
- `GET /api/projects/{name}/tickets/{id}/output` (SSE)
- `POST /api/projects/{name}/tickets/{id}/kill`
- `POST /api/projects/{name}/tickets/{id}/respawn`
- `POST /api/projects/{name}/tickets/{id}/done`
- `POST /api/projects/{name}/tickets/{id}/fail`
- `GET /api/projects/{name}/phase-gate`
- `POST /api/projects/{name}/phase-gate/approve`
- `GET /api/watchdog/status`
- `GET /api/watchdog/log`
- `POST /api/watchdog/run`
- `GET /api/health`
- `GET /api/events` (SSE event bus)

SSE event bus constants:
- `ticket_done`, `ticket_spawned`, `progress`, `phase_gate`, `failure`, `ram_warning`

## Command Surface (Current)

- `swarm init`
- `swarm add-ticket`
- `swarm prompts check|gen`
- `swarm status`
- `swarm watch`
- `swarm go`
- `swarm integrate`
- `swarm archive`
- `swarm cleanup`
- `swarm serve`
- `swarm install`

Global flag: `--config`.

## File Layout

```text
<project>/
  swarm.toml
  AGENTS.md
  .agents/
    skills/
    profiles/
  .codex/
    rules/
  swarm/
    tracker.json
    archive.json
    events.jsonl
    prompt-footer.md
    prompts/
      <ticket>.md
    features/
    logs/

<project>-worktrees/
  <ticket>/
    .codex-prompt.md
```

## Quality Gates (Developer)

```bash
go test ./... -count=1
go build ./...
go vet ./...
```
