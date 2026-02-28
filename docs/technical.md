# Agent Swarm — Technical Documentation

## Architecture

```
                    ┌─────────────┐
                    │   CLI (cmd/) │
                    └──────┬──────┘
                           │
          ┌────────────────┼────────────────┐
          │                │                │
   ┌──────▼──────┐ ┌──────▼──────┐ ┌───────▼──────┐
   │   config/   │ │  tracker/   │ │  dispatcher/ │
   │ swarm.toml  │ │ tracker.json│ │ phase logic  │
   └──────┬──────┘ └──────┬──────┘ └───────┬──────┘
          │                │                │
   ┌──────▼──────┐ ┌──────▼──────┐ ┌───────▼──────┐
   │  backend/   │ │  worktree/  │ │  watchdog/   │
   │ codex tmux  │ │ git mgmt    │ │ health loop  │
   └──────┬──────┘ └─────────────┘ └───────┬──────┘
          │                                │
   ┌──────▼──────┐                  ┌──────▼──────┐
   │  progress/  │                  │   notify/   │
   │ markers +   │                  │ stdout/tg   │
   │ heuristics  │                  └─────────────┘
   └─────────────┘
          │
   ┌──────▼──────┐   ┌─────────────┐
   │    tui/     │   │   server/   │
   │ bubbletea   │   │ HTTP + SSE  │
   └─────────────┘   └─────────────┘
```

## Package Reference

### `internal/config`

Loads and validates `swarm.toml`.

```go
type Config struct {
    Project       ProjectConfig
    Backend       BackendConfig
    Notifications NotificationsConfig
    Watchdog      WatchdogConfig
    Integration   IntegrationConfig
    Serve         ServeConfig
    Install       InstallConfig
}

func Load(path string) (*Config, error)
func Default() *Config
func (c *Config) Validate() error
```

**Config fields:**

| Section | Field | Type | Default | Description |
|---|---|---|---|---|
| project | name | string | required | Project identifier |
| project | repo | string | "." | Repository root path |
| project | base_branch | string | "main" | Branch to create worktrees from |
| project | max_agents | int | 7 | Maximum concurrent agents |
| project | min_ram_mb | int | 1024 | Minimum free RAM to spawn |
| project | prompt_dir | string | "swarm/prompts" | Prompt file directory |
| project | tracker | string | "swarm/tracker.json" | Tracker file path |
| backend | type | string | "codex-tmux" | Agent backend type |
| backend | model | string | "" | LLM model identifier |
| backend | binary | string | auto-detect | Path to codex binary |
| backend | bypass_sandbox | bool | true | Skip sandbox restrictions |
| watchdog | interval | string | "5m" | Check interval (Go duration) |
| watchdog | max_runtime | string | "45m" | Alert if agent runs longer |
| watchdog | max_retries | int | 2 | Respawn attempts before failing |
| integration | verify_cmd | string | "" | Build verification command |
| integration | audit_ticket | string | "" | Auto-spawn after integration |

### `internal/tracker`

Manages ticket state. The tracker JSON file is the single source of truth.

```go
type Tracker struct {
    Project  string            `json:"project"`
    Tickets  map[string]Ticket `json:"tickets"`
}

type Ticket struct {
    Status  string   `json:"status"`
    Phase   int      `json:"phase"`
    Depends []string `json:"depends,omitempty"`
    Branch  string   `json:"branch"`
    Desc    string   `json:"desc"`
    SHA     string   `json:"sha,omitempty"`
}

func Load(path string) (*Tracker, error)
func (t *Tracker) Save(path string) error
func (t *Tracker) GetSpawnable() []string
func (t *Tracker) ActivePhase() int
func (t *Tracker) Stats() Stats
func (t *Tracker) DependencyOrder() []string   // topological sort
func (t *Tracker) PhaseNumbers() []int
```

**Status lifecycle:** `todo` → `running` → `done` | `failed`

**Spawnable logic:** A ticket is spawnable when:
1. Status is `todo`
2. All dependencies have status `done`
3. Phase ≤ active phase (lowest phase with non-done tickets)

**Dependency order:** Topological sort using Kahn's algorithm. Used by `swarm integrate` to merge branches in safe order (leaves first, dependents last).

### `internal/dispatcher`

Phase-gated spawn logic with dependency resolution.

```go
type Signal string
const (
    SignalSpawn     Signal = ""
    SignalPhaseGate Signal = "PHASE_GATE"
    SignalAllDone   Signal = "ALL_DONE"
    SignalBlocked   Signal = "BLOCKED"
)

type Dispatcher struct { ... }

func New(cfg *config.Config, t *tracker.Tracker) *Dispatcher
func (d *Dispatcher) Evaluate() (Signal, []string)
func (d *Dispatcher) CanSpawnMore() bool
func (d *Dispatcher) NextSpawnable(n int) []string
func (d *Dispatcher) MarkDone(ticketID, sha string) (Signal, []string)
func (d *Dispatcher) MarkFailed(ticketID string) error
func (d *Dispatcher) ApprovePhaseGate() (Signal, []string)
func (d *Dispatcher) CurrentPhase() int
func (d *Dispatcher) PhaseStatus() PhaseStatus
```

**Signal decision logic:**
```
if all tickets done → ALL_DONE
if current phase complete AND next phase exists → PHASE_GATE
if spawnable tickets exist AND under max_agents AND RAM OK → (spawn)
otherwise → BLOCKED
```

**MarkDone chain:** When a ticket completes, the dispatcher recalculates spawnable tickets. Newly unblocked tickets are returned so the watchdog can spawn them.

### `internal/backend`

Agent lifecycle management. Currently supports Codex via tmux.

```go
type AgentBackend interface {
    Spawn(ctx context.Context, cfg SpawnConfig) (AgentHandle, error)
    IsAlive(handle AgentHandle) bool
    HasExited(handle AgentHandle) bool
    GetOutput(handle AgentHandle, lines int) (string, error)
    Kill(handle AgentHandle) error
    Name() string
}

type SpawnConfig struct {
    TicketID   string
    Branch     string
    WorkDir    string
    PromptFile string
    Model      string
    Effort     string
}

type AgentHandle struct {
    SessionName string
    PID         int
    StartedAt   time.Time
}
```

**Codex tmux implementation:**
- `Spawn`: Creates tmux session `swarm-<ticketID>`, runs `codex exec -m <model> --dangerously-bypass-approvals-and-sandbox -C <dir> "<prompt>"`
- `IsAlive`: `tmux has-session -t <session>` returns 0
- `HasExited`: Session exists but shell process is gone
- `GetOutput`: `tmux capture-pane -t <session> -p -S -<lines>`
- `Kill`: `tmux kill-session -t <session>`
- `ListSessions`: `tmux list-sessions` filtered by `swarm-` prefix

### `internal/watchdog`

The core orchestration loop.

```go
type Watchdog struct { ... }

func New(cfg, tracker, dispatcher, backend, worktree, notifier, eventLog) *Watchdog
func (w *Watchdog) Run(ctx context.Context) error       // loop with interval
func (w *Watchdog) RunOnce(ctx context.Context) error    // single pass
func (w *Watchdog) SpawnTicket(ctx context.Context, ticketID string) error
```

**RunOnce algorithm:**
```
1. For each ticket with status "running":
   a. Check if agent tmux session still alive
   b. If exited with commits:
      - dispatcher.MarkDone(ticket, sha)
      - Spawn newly unblocked tickets
   c. If exited without commits (attempt 1):
      - Respawn the agent
   d. If exited without commits (attempt 2+):
      - dispatcher.MarkFailed(ticket)
      - Notify
   e. If alive > max_runtime:
      - Notify "possibly stuck"

2. If 0 agents running AND spawnable tickets exist AND RAM OK:
   - Idle auto-spawn (up to max_agents)

3. If phase gate reached:
   - Notify, stop spawning
```

**SpawnTicket flow:**
1. Create worktree via worktree.Manager
2. Copy prompt file to worktree as `.codex-prompt.md`
3. Append prompt footer if exists
4. Call backend.Spawn()
5. Update tracker status to "running"

**Event log:** Appends JSON lines to `events.jsonl`:
```json
{"type":"ticket_done","ticket":"sw-01","timestamp":"2026-02-28T12:00:00Z","data":{"sha":"abc123"}}
{"type":"ticket_spawned","ticket":"sw-05","timestamp":"2026-02-28T12:00:01Z","data":{}}
{"type":"phase_gate","ticket":"","timestamp":"2026-02-28T12:00:02Z","data":{"phase":2}}
```

### `internal/worktree`

Git worktree lifecycle.

```go
type Manager struct { ... }

func New(repoDir, worktreeBase, baseBranch string) *Manager
func (m *Manager) Create(ticketID, branch string) (string, error)
func (m *Manager) Remove(ticketID string) error
func (m *Manager) Prune() error
func (m *Manager) List() ([]Worktree, error)
func (m *Manager) HasCommits(ticketID, baseBranch string) (bool, string, error)
func (m *Manager) CleanupOlderThan(duration time.Duration) ([]string, error)
```

Worktrees are created at `<worktreeBase>/<ticketID>/` with branch `feat/<ticketID>` from `baseBranch`.

### `internal/progress`

Agent progress detection.

```go
func ParseMarker(output string) *Marker           // find PROGRESS: X/N
func InferHeuristic(output string, runtime time.Duration) int  // 0-100
func GetProgress(handle, backend, promptTasks) TicketProgress
```

**Marker format:** `PROGRESS: 3/6` (on its own line, last match wins)

**Heuristic signals:**
| Signal | % |
|---|---|
| Agent started, 0 files | 5 |
| Files created | 30 |
| Build/test output | 70 |
| `git commit` in output | 90 |
| `git push` in output | 95 |
| Exited with commits | 100 |

### `internal/tui`

Bubbletea terminal UI for `swarm status --watch`.

Renders: overall progress bar, per-ticket rows with status icons and progress bars, RAM/agent counts. Refreshes every 3 seconds.

### `internal/server`

HTTP API + SSE for web dashboard integration.

**Endpoints:**

| Method | Path | Description |
|---|---|---|
| GET | /api/health | System health (RAM, agents, uptime) |
| GET | /api/projects | List projects |
| GET | /api/projects/:name/status | Full dispatcher status |
| GET | /api/projects/:name/tickets | All tickets with progress |
| GET | /api/projects/:name/tickets/:id | Single ticket detail |
| GET | /api/projects/:name/tickets/:id/output | SSE: live agent output |
| POST | /api/projects/:name/tickets/:id/kill | Kill agent |
| POST | /api/projects/:name/tickets/:id/respawn | Respawn agent |
| POST | /api/projects/:name/tickets/:id/done | Manual mark done |
| POST | /api/projects/:name/tickets/:id/fail | Manual mark failed |
| GET | /api/projects/:name/phase-gate | Phase gate status |
| POST | /api/projects/:name/phase-gate/approve | Approve gate |
| GET | /api/watchdog/status | Watchdog health |
| POST | /api/watchdog/run | Trigger watchdog pass |
| GET | /api/events | SSE: all events |

**SSE event types:** `ticket_done`, `ticket_spawned`, `progress`, `phase_gate`, `failure`, `ram_warning`

**Auth:** Optional bearer token via `[serve].auth_token` in config. Empty = no auth.

### `internal/notify`

Notification adapters.

```go
type Notifier interface {
    Alert(ctx context.Context, msg string) error
    Info(ctx context.Context, msg string) error
}
```

Implementations: `StdoutNotifier` (default), `TelegramNotifier` (via Bot API).

### `internal/sysinfo`

System resource checks.

```go
func AvailableRAM() (int, error)    // MB, from /proc/meminfo
func CanSpawn(minRAM int) bool
```

## Data Flow

```
Ticket defined (add-ticket)
    ↓
Prompt written (prompts gen)
    ↓
Watchdog detects spawnable ticket
    ↓
Worktree created (git worktree add)
    ↓
Prompt copied to worktree
    ↓
Agent spawned in tmux (codex exec)
    ↓
Watchdog polls: alive? exited?
    ↓
Agent exits with commits
    ↓
Dispatcher marks done, calculates newly unblocked
    ↓
Watchdog spawns next tickets
    ↓
Phase complete → PHASE_GATE signal
    ↓
Human approves (swarm go)
    ↓
Next phase auto-spawns
    ↓
ALL_DONE → swarm integrate → merge branches
```

## File Layout

```
swarm.toml                      # project config
swarm/
├── tracker.json                # ticket state (source of truth)
├── prompts/                    # one .md per ticket
│   ├── feat-01.md
│   └── feat-02.md
├── prompt-footer.md            # appended to all prompts (optional)
└── events.jsonl                # watchdog event log

<project>-worktrees/            # sibling directory
├── feat-01/                    # isolated git worktree
├── feat-02/
└── ...
```

## Testing

```bash
go test ./... -v -count=1       # all tests
go test ./internal/tracker/...  # single package
go test ./... -cover            # with coverage
go vet ./...                    # static analysis
```

Test strategy:
- **Config:** Valid/invalid TOML parsing, defaults, validation
- **Tracker:** CRUD, spawnable resolution, topological sort
- **Dispatcher:** Phase gates, signal evaluation, MarkDone chains
- **Watchdog:** Mock backend simulating agent exits with/without commits
- **Worktree:** Temp git repos for create/remove/hasCommits
- **Server:** httptest handlers, SSE format, auth middleware
- **Progress:** Marker parsing, heuristic inference
- **TUI:** Model update with simulated key messages

## Extending

**New backend:** Implement the `AgentBackend` interface in `internal/backend/`. Register in `cmd/helpers.go` `buildBackend()`.

**New notifier:** Implement `Notifier` interface in `internal/notify/`. Register in `buildNotifier()`.

**New commands:** Add a file in `cmd/`, create a cobra command, register in `init()`.
