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
3. Exited tickets are reconciled via runtime exit markers + backend checks.
4. Completion requires meaningful diff + verify pass + guardian allow.
5. Tickets become done/failed deterministically and events are recorded.
6. Dispatcher evaluates signal (`spawn`, `PHASE_GATE`, `ALL_DONE`, `BLOCKED`).
7. Spawnable tickets are launched up to capacity constraints.
8. Tracker and `events.jsonl` are updated.

## v2 Prompt Assembly

At spawn time, each ticket prompt is assembled by `watchdog.assemblePrompt`:

1. `AGENTS.md` (project governance)
2. `project.spec_file` (optional)
3. profile markdown (`ticket.profile`)
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
- `post_build.order` (default doc-only)
- `post_build.require_integrated_base`
- `post_build.integrated_base_branch`
- `guardian.enabled`
- `guardian.flow_file`
- `guardian.mode` (`advisory` | `enforce`)

### `internal/tracker`

Tracker JSON is the state source of truth.

`Ticket` fields include:
- `status`, `phase`, `depends`, `branch`, `desc`
- `profile` (explicit role, required in strict mode)
- `verify_cmd`, `priority`
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
- Spawn ordering is priority-aware (`ticket.priority`), then lexical fallback.
- `swarm plan optimize` computes throughput-oriented priorities from DAG structure.

### `internal/watchdog`

Responsibilities:
- Spawn tickets via worktree + backend.
- Retry failed exits without commits.
- Mark tickets failed after retry limit.
- Detect long-running tickets via `max_runtime`.
- Gate post-build spawns on integrated baseline merge (default base: `dev`).
- Persist JSONL events to `<state_dir>/events.jsonl` and run retention maintenance.

Watchdog event types emitted to JSONL include:
- `ticket_done`
- `respawn`
- `ticket_failed`
- `verify_failed_respawn`
- `idle_spawn`
- `phase_gate_auto_approved`
- `phase_gate`
- `project_complete`
- `ticket_spawned`
- `guardian_block`

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

### `cmd/watchdog_cmd.go`

- `watchdog run-all-once` executes one pass for every project in registry (`projects.json`).
- Used by scheduled installs so monitoring continues across all initialized projects.

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
- `swarm prompts check`
- `swarm status`
- `swarm prep`
- `swarm doctor`
- `swarm plan optimize`
- `swarm watch`
- `swarm go`
- `swarm integrate`
- `swarm archive`
- `swarm cleanup`
- `swarm guardian report`
- `swarm guardian migrate`
- `swarm serve`
- `swarm install` (runs multi-project watchdog runner)

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
    tracker.seed.json
    archive.json
    prompt-footer.md
    prompts/
      <ticket>.md
    features/

  .local/state/
    tracker.json
    events.jsonl
    rollups/YYYY-MM-DD.json
    runs/<ticket>/spawn.json
    runs/<ticket>/exit.json

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

## Runtime Refresh Contract

After every patch/rebuild, refresh runtime deterministically:

```bash
scripts/refresh-runtime.sh
```

Contract enforced by script:
- canonical runtime binary: `~/.local/bin/agent-swarm`
- watchdog service ExecStart points to canonical binary
- user systemd daemon reload + timer restart
- binary/service hash parity verification
