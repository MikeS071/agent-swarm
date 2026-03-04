package watchdog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/notify"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

const swarmSessionPrefix = "swarm-"

const defaultPromptFooter = `
---

## MANDATORY DEVELOPMENT PROCESS (appended automatically — follow in exact order)

### Phase 1: Understand the spec
- Read the task objective and requirements above
- Identify every behaviour, input, output, and error case

### Phase 2: Write tests FIRST (before any implementation code)
- Write failing tests that define the expected behaviour from the spec
- Min 3 test cases per function: happy path, error path, edge case
- Table-driven tests where applicable
- Mock external dependencies (DB, HTTP, Docker) — no real connections
- Run tests — they SHOULD fail (red). This confirms they test real behaviour.

### Phase 3: Implement
- Write the minimum code to make all tests pass
- Do NOT write code that isn't covered by a test

### Phase 4: Quality gates (run in order, fix and re-run from gate 1 on failure)
1. **Tests pass:** ` + "`go test ./... -count=1`" + ` (Go) or ` + "`pnpm test`" + ` (Web/TS)
2. **Build passes:** ` + "`go build ./...`" + ` (Go) or ` + "`pnpm build`" + ` (Web/TS)
3. **Types/Lint:** ` + "`go vet ./...`" + ` (Go) or ` + "`pnpm typecheck`" + ` (Web/TS)
4. Iterate until all green

### Phase 5: Commit (only after ALL gates pass)
` + "```bash" + `
git add -A
git commit -m "feat: <description>

Tests: X passed, 0 failed"
git push origin HEAD
` + "```" + `

Do NOT exit without committing and pushing.
Do NOT commit if any gate is failing.
Do NOT write implementation before tests.
`

// Event is a single append-only watchdog event log entry.
type Event struct {
	Type      string         `json:"type"`
	Ticket    string         `json:"ticket,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// EventLog appends watchdog events as JSON lines.
type EventLog struct {
	path string
	mu   sync.Mutex
}

type reviewReport struct {
	Findings []reviewFinding `json:"findings"`
}

type reviewFinding struct {
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SuggestedFix string `json:"suggested_fix"`
}

// NewEventLog creates an append-only event log writer.
func NewEventLog(path string) *EventLog {
	return &EventLog{path: path}
}

// Append writes an event as one JSON object per line.
func (l *EventLog) Append(ev Event) error {
	if l == nil || strings.TrimSpace(l.path) == "" {
		return nil
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("mkdir event log dir: %w", err)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write event log: %w", err)
	}
	return nil
}

// Watchdog runs the self-healing watch loop.
type Watchdog struct {
	configPath string
	config     *config.Config
	tracker    *tracker.Tracker
	dispatcher *dispatcher.Dispatcher
	backend    backend.AgentBackend
	worktree   *worktree.Manager
	notifier   notify.Notifier
	events     *EventLog
	guardian   guardian.Evaluator

	backendFactory func(backendType string) (backend.AgentBackend, error)
	backendCache   map[string]backend.AgentBackend
	backendMu      sync.Mutex

	dryRun         bool
	retries        map[string]int
	spawnErrors    map[string]int
	stuckAlerts    map[string]bool
	gateNoticed    bool
	completionSent bool
	logger         *log.Logger
}

// New creates a watchdog instance.
func New(
	cfg *config.Config,
	tr *tracker.Tracker,
	d *dispatcher.Dispatcher,
	be backend.AgentBackend,
	wt *worktree.Manager,
	n notify.Notifier,
) *Watchdog {
	if cfg == nil {
		cfg = config.Default()
	}
	if n == nil {
		n = notify.NewStdoutNotifier(nil)
	}
	if d == nil {
		d = dispatcher.New(cfg, tr)
	}
	eventsPath := ""
	if cfg != nil && strings.TrimSpace(cfg.Project.Tracker) != "" {
		eventsPath = filepath.Join(filepath.Dir(cfg.Project.Tracker), "events.jsonl")
	}

	cache := map[string]backend.AgentBackend{}
	defaultType := normalizedBackendType("")
	if cfg != nil {
		defaultType = normalizedBackendType(cfg.Backend.Type)
	}
	if be != nil {
		cache[defaultType] = be
	}

	w := &Watchdog{
		config:       cfg,
		tracker:      tr,
		dispatcher:   d,
		backend:      be,
		worktree:     wt,
		notifier:     n,
		events:       NewEventLog(eventsPath),
		guardian:     guardian.NoopEvaluator{},
		backendCache: cache,
		retries:      map[string]int{},
		spawnErrors:  map[string]int{},
		stuckAlerts:  map[string]bool{},
	}
	w.backendFactory = func(backendType string) (backend.AgentBackend, error) {
		if w.config == nil {
			return backend.Build(backendType, backend.BuildOptions{})
		}
		return backend.Build(backendType, backend.BuildOptions{
			Binary:        w.config.Backend.Binary,
			BypassSandbox: w.config.Backend.BypassSandbox,
		})
	}
	return w
}

// SetDryRun toggles non-mutating evaluation mode.

func (w *Watchdog) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if w.logger != nil {
		w.logger.Println(msg)
	} else {
		fmt.Fprintf(os.Stderr, "[watchdog] %s\n", msg)
	}
}

func (w *Watchdog) SetDryRun(v bool) {
	if w != nil {
		w.dryRun = v
	}
}

func (w *Watchdog) SetConfigPath(p string) {
	if w != nil {
		w.configPath = p
	}
}

func (w *Watchdog) SetGuardian(g guardian.Evaluator) {
	if w == nil {
		return
	}
	if g == nil {
		w.guardian = guardian.NoopEvaluator{}
		return
	}
	w.guardian = g
}

// Run executes the watchdog loop at configured interval until ctx cancellation.
func (w *Watchdog) ReconcileRunning(ctx context.Context) error {
	if w == nil || w.tracker == nil {
		return nil
	}
	for _, ticketID := range runningTicketIDs(w.tracker) {
		tk := w.tracker.Tickets[ticketID]
		handle := w.sessionHandleForTicket(ticketID, tk)
		runningBackend, _, err := w.runtimeBackendForTicket(tk)
		if err != nil {
			continue
		}
		exitMeta, hasExitMarker := w.readExitMarker(ticketID)
		if !(hasExitMarker || runningBackend.HasExited(handle)) {
			continue
		}
		hasCommits, sha, err := w.worktree.HasCommits(ticketID, w.config.Project.BaseBranch)
		if err != nil {
			hasCommits = false
		}
		if hasCommits {
			gdec, _ := w.guardianCheck(ctx, guardian.EventBeforeMarkDone, ticketID, tk)
			if gdec.Result == guardian.ResultBlock {
				_ = w.dispatcher.MarkFailed(ticketID)
				_ = w.saveTracker()
				continue
			}
			if ok, _ := w.verifyTicket(ticketID, tk); ok {
				_, _ = w.dispatcher.MarkDone(ticketID, sha)
				delete(w.retries, ticketID)
				d := map[string]any{"sha": sha}
				if hasExitMarker {
					d["process_exit_code"] = exitMeta.ProcessExitCode
				}
				_ = w.appendEvent("ticket_done", ticketID, d)
				_ = w.saveTracker()
				continue
			}
		}
		_ = w.dispatcher.MarkFailed(ticketID)
		_ = w.saveTracker()
		d := map[string]any{"reason": "reconcile_failed"}
		if hasExitMarker {
			d["process_exit_code"] = exitMeta.ProcessExitCode
		}
		_ = w.appendEvent("ticket_failed", ticketID, d)
	}
	return nil
}

func (w *Watchdog) Run(ctx context.Context) error {
	interval := 5 * time.Minute
	if parsed, err := time.ParseDuration(strings.TrimSpace(w.config.Watchdog.Interval)); err == nil && parsed > 0 {
		interval = parsed
	}

	if err := w.RunOnce(ctx); err != nil {
		w.log("ERROR: initial watchdog pass failed: %v", err)
		// Continue to ticker loop — don't exit
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log("ERROR: watchdog pass failed: %v", err)
				// Continue running — don't crash the loop
			}
		}
	}
}

// RunOnce executes one watchdog pass.
func (w *Watchdog) RunOnce(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("watchdog is nil")
	}
	if w.backend == nil || w.tracker == nil || w.dispatcher == nil || w.worktree == nil {
		return fmt.Errorf("watchdog dependencies are not initialized")
	}

	// Re-read auto_approve from config file (TUI 'm' key writes to swarm.toml).
	if w.configPath != "" {
		if freshCfg, err := config.Load(w.configPath); err == nil {
			w.config.Project.AutoApprove = freshCfg.Project.AutoApprove
		}
	}

	// Re-read tracker from disk and merge externally-added tickets to avoid clobbering.
	if w.config != nil && w.config.Project.Tracker != "" {
		if fresh, err := tracker.Load(w.config.Project.Tracker); err == nil {
			if w.tracker != nil {
				if w.tracker.Tickets == nil {
					w.tracker.Tickets = map[string]tracker.Ticket{}
				}
				for id, tk := range fresh.Tickets {
					if _, ok := w.tracker.Tickets[id]; !ok {
						w.tracker.Tickets[id] = tk
					}
				}
				if fresh.UnlockedPhase > w.tracker.UnlockedPhase {
					w.tracker.UnlockedPhase = fresh.UnlockedPhase
					if w.dispatcher != nil {
						w.dispatcher.SetUnlockedPhase(fresh.UnlockedPhase)
					}
				}
			}
		} else {
			w.log("WARN: reload tracker from disk failed: %v", err)
		}
	}

	if _, err := w.listRunningSessions(ctx); err != nil {
		w.log("WARN: listRunningSessions: %v", err)
	}

	w.log("pass: %d running, checking exits", len(runningTicketIDs(w.tracker)))
	for _, ticketID := range runningTicketIDs(w.tracker) {
		tk := w.tracker.Tickets[ticketID]
		handle := w.sessionHandleForTicket(ticketID, tk)
		runningBackend, _, err := w.runtimeBackendForTicket(tk)
		if err != nil {
			w.log("WARN: backend for %s: %v", ticketID, err)
			continue
		}

		exitMeta, hasExitMarker := w.readExitMarker(ticketID)
		if hasExitMarker || runningBackend.HasExited(handle) {
			hasCommits, sha, err := w.worktree.HasCommits(ticketID, w.config.Project.BaseBranch)
			if err != nil {
				w.log("WARN: HasCommits(%s) error: %v — treating as no commits", ticketID, err)
				hasCommits = false
			}

			if hasCommits {
				if w.dryRun {
					_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would mark %s done (%s)", ticketID, sha))
					continue
				}
				gdec, _ := w.guardianCheck(ctx, guardian.EventBeforeMarkDone, ticketID, tk)
				if gdec.Result == guardian.ResultBlock {
					_ = w.dispatcher.MarkFailed(ticketID)
					_ = w.saveTracker()
					_ = w.appendEvent("guardian_block", ticketID, map[string]any{"event": "before_mark_done", "rule": gdec.RuleID, "reason": gdec.Reason, "evidence": gdec.EvidencePath})
					_ = w.notifier.Alert(ctx, fmt.Sprintf("guardian blocked completion for %s", ticketID))
					continue
				}
				if ok, vErr := w.verifyTicket(ticketID, tk); !ok {
					w.retries[ticketID]++
					attempt := w.retries[ticketID]
					if attempt < w.maxRetries() {
						data := map[string]any{"attempt": attempt, "error": errString(vErr)}
						if hasExitMarker {
							data["process_exit_code"] = exitMeta.ProcessExitCode
						}
						_ = w.appendEvent("verify_failed_respawn", ticketID, data)
						if err := w.SpawnTicket(ctx, ticketID); err != nil {
							w.log("WARN: respawn after verify fail (%s): %v", ticketID, err)
						}
						continue
					}
					_ = w.dispatcher.MarkFailed(ticketID)
					_ = w.saveTracker()
					data := map[string]any{"reason": "verify_failed", "error": errString(vErr), "attempt": attempt}
					if hasExitMarker {
						data["process_exit_code"] = exitMeta.ProcessExitCode
					}
					_ = w.appendEvent("ticket_failed", ticketID, data)
					_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s failed verification", ticketID))
					continue
				}
				sig, spawnable := w.dispatcher.MarkDone(ticketID, sha)
				delete(w.retries, ticketID)
				if created, err := w.autoCreateFixTickets(ticketID); err != nil {
					w.log("WARN: autoCreateFixTickets(%s): %v", ticketID, err)
				} else if created > 0 {
					sig, spawnable = w.dispatcher.Evaluate()
				}
				// Clean up agent log on successful completion
				if w.config != nil && w.config.Project.Tracker != "" {
					logFile := filepath.Join(filepath.Dir(w.config.Project.Tracker), "logs", ticketID+".log")
					os.Remove(logFile)
				}
				doneData := map[string]any{"sha": sha}
				if hasExitMarker {
					doneData["process_exit_code"] = exitMeta.ProcessExitCode
				}
				if err := w.appendEvent("ticket_done", ticketID, doneData); err != nil {
					w.log("WARN: appendEvent(ticket_done, %s): %v", ticketID, err)
				}
				if err := w.saveTracker(); err != nil {
					w.log("WARN: saveTracker: %v", err)
				}
				if sig == dispatcher.SignalSpawn && w.dispatcher.CanSpawnMore() && len(spawnable) > 0 {
					if err := w.SpawnTicket(ctx, spawnable[0]); err != nil {
						w.log("WARN: SpawnTicket(%s) after done: %v", spawnable[0], err)
					}
				}
				continue
			}

			w.retries[ticketID]++
			attempt := w.retries[ticketID]
			if attempt < w.maxRetries() {
				if w.dryRun {
					_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would respawn %s (attempt %d)", ticketID, attempt))
					continue
				}
				data := map[string]any{"attempt": attempt}
				if hasExitMarker {
					data["process_exit_code"] = exitMeta.ProcessExitCode
				}
				if err := w.appendEvent("respawn", ticketID, data); err != nil {
					w.log("WARN: appendEvent(respawn, %s): %v", ticketID, err)
				}
				if err := w.SpawnTicket(ctx, ticketID); err != nil {
					w.log("WARN: respawn SpawnTicket(%s): %v", ticketID, err)
				}
				continue
			}

			if w.dryRun {
				_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would mark %s failed after %d attempts", ticketID, attempt))
				continue
			}
			if err := w.dispatcher.MarkFailed(ticketID); err != nil {
				w.log("WARN: MarkFailed(%s): %v", ticketID, err)
			}
			if err := w.saveTracker(); err != nil {
				return err
			}
			data := map[string]any{"attempt": attempt}
			if hasExitMarker {
				data["process_exit_code"] = exitMeta.ProcessExitCode
			}
			if err := w.appendEvent("ticket_failed", ticketID, data); err != nil {
				return err
			}
			_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s failed after %d attempts", ticketID, attempt))
			continue
		}

		if runningBackend.IsAlive(handle) && w.runtimeExceeded(tk.StartedAt) && !w.stuckAlerts[ticketID] {
			w.stuckAlerts[ticketID] = true
			_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s may be stuck (exceeded max_runtime)", ticketID))
		}
	}

	if err := w.ensurePostBuildTickets(ctx); err != nil {
		w.log("WARN: ensurePostBuildTickets: %v", err)
	}

	runningCount, err := w.runningAgentCount(ctx)
	if err != nil {
		w.log("WARN: runningAgentCount: %v — assuming 0", err)
		runningCount = 0
	}
	{
		sig, spawnable := w.dispatcher.Evaluate()
		w.log("idle check: signal=%s, spawnable=%d, running=%d", sig, len(spawnable), runningCount)
		if sig == dispatcher.SignalSpawn && len(spawnable) > 0 && w.dispatcher.CanSpawnMore() {
			slots := w.config.Project.MaxAgents - runningCount
			if slots <= 0 {
				slots = 1
			}
			if slots > len(spawnable) {
				slots = len(spawnable)
			}
			for i := 0; i < slots; i++ {
				if !w.dispatcher.CanSpawnMore() {
					w.log("capacity exhausted, deferring remaining spawns")
					break
				}
				ticketID := spawnable[i]
				if w.dryRun {
					_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would idle-spawn %s", ticketID))
					continue
				}
				if err := w.appendEvent("idle_spawn", ticketID, nil); err != nil {
					w.log("WARN: appendEvent(idle_spawn, %s): %v", ticketID, err)
				}
				if err := w.SpawnTicket(ctx, ticketID); err != nil {
					w.spawnErrors[ticketID]++
					w.log("WARN: idle SpawnTicket(%s): %v (attempt %d/3)", ticketID, err, w.spawnErrors[ticketID])
					if w.spawnErrors[ticketID] >= 3 {
						w.log("ERROR: marking %s failed after 3 spawn failures", ticketID)
						_ = w.dispatcher.MarkFailed(ticketID)
						_ = w.saveTracker()
						_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s failed: spawn error after 3 attempts", ticketID))
					}
				} else {
					delete(w.spawnErrors, ticketID)
				}
			}
		}
	}

	sig, _ := w.dispatcher.Evaluate()
	if sig == dispatcher.SignalPhaseGate {
		gdec, _ := w.guardianCheck(ctx, guardian.EventPhaseTransition, "", tracker.Ticket{Phase: w.dispatcher.CurrentPhase()})
		if gdec.Result == guardian.ResultBlock {
			if !w.gateNoticed {
				w.gateNoticed = true
				_ = w.appendEvent("guardian_block", "", map[string]any{"event": "phase_transition", "rule": gdec.RuleID, "reason": gdec.Reason, "evidence": gdec.EvidencePath, "phase": w.dispatcher.CurrentPhase()})
				_ = w.notifier.Alert(ctx, fmt.Sprintf("guardian blocked phase transition: %s", gdec.Reason))
			}
			return nil
		}
		if w.config.Project.AutoApprove {
			_ = w.notifier.Info(ctx, "phase gate reached — auto-approving")
			approvedSig, spawnable := w.dispatcher.ApprovePhaseGate()
			if err := w.saveTracker(); err != nil {
				w.log("WARN: saveTracker after phase gate: %v", err)
			}
			if err := w.appendEvent("phase_gate_auto_approved", "", nil); err != nil {
				w.log("WARN: appendEvent(phase_gate_auto_approved): %v", err)
			}
			if approvedSig == dispatcher.SignalSpawn && len(spawnable) > 0 && w.dispatcher.CanSpawnMore() {
				slots := w.config.Project.MaxAgents
				if slots <= 0 {
					slots = 1
				}
				if slots > len(spawnable) {
					slots = len(spawnable)
				}
				for i := 0; i < slots; i++ {
					if err := w.SpawnTicket(ctx, spawnable[i]); err != nil {
						w.log("WARN: gate SpawnTicket(%s): %v", spawnable[i], err)
					}
				}
			}
			w.gateNoticed = false
		} else if !w.gateNoticed {
			w.gateNoticed = true
			if w.dryRun {
				_ = w.notifier.Info(ctx, "[dry-run] phase gate reached")
			} else {
				if err := w.appendEvent("phase_gate", "", nil); err != nil {
					w.log("WARN: appendEvent(phase_gate): %v", err)
				}
				_ = w.notifier.Info(ctx, "phase gate reached; run `swarm go` to continue")
			}
		}
	} else {
		w.gateNoticed = false
	}

	if sig == dispatcher.SignalAllDone && !w.completionSent {
		gdec, _ := w.guardianCheck(ctx, guardian.EventPostBuildDone, "", tracker.Ticket{Phase: w.dispatcher.CurrentPhase()})
		if gdec.Result == guardian.ResultBlock {
			_ = w.appendEvent("guardian_block", "", map[string]any{"event": "post_build_complete", "rule": gdec.RuleID, "reason": gdec.Reason, "evidence": gdec.EvidencePath})
			_ = w.notifier.Alert(ctx, fmt.Sprintf("guardian blocked completion: %s", gdec.Reason))
			return nil
		}
		signature := w.completionSignature()
		if w.wasCompletionNotified(signature) {
			w.completionSent = true
		} else {
			w.completionSent = true
			report := w.buildCompletionReport()
			if err := w.appendEvent("project_complete", "", nil); err != nil {
				w.log("WARN: appendEvent(project_complete): %v", err)
			}
			_ = w.notifier.Alert(ctx, report)
			if err := w.markCompletionNotified(signature); err != nil {
				w.log("WARN: markCompletionNotified: %v", err)
			}
		}
	}

	if err := w.maybeSendStatusReport(ctx, sig); err != nil {
		w.log("WARN: status report: %v", err)
	}
	w.runTelemetryMaintenance(time.Now().UTC())

	return nil
}

func (w *Watchdog) maybeSendStatusReport(ctx context.Context, sig dispatcher.Signal) error {
	if w == nil || w.config == nil || w.tracker == nil {
		return nil
	}
	cfg := w.config.StatusReport
	if !cfg.Enabled {
		return nil
	}
	now := time.Now().UTC()
	interval := 5 * time.Minute
	if d, err := time.ParseDuration(strings.TrimSpace(cfg.Interval)); err == nil && d > 0 {
		interval = d
	}
	stats := w.tracker.Stats()
	running := stats.Running
	if cfg.OnlyWhenRunning && running == 0 {
		return nil
	}

	last, _ := w.readLastStatusReportAt()
	if !last.IsZero() && now.Sub(last) < interval {
		return nil
	}

	active := runningTicketIDs(w.tracker)
	sort.Strings(active)
	if len(active) > 5 {
		active = active[:5]
	}
	msg := fmt.Sprintf("%s: done=%d running=%d todo=%d failed=%d", w.tracker.Project, stats.Done, stats.Running, stats.Todo, stats.Failed)
	if len(active) > 0 {
		msg += " | active: " + strings.Join(active, ", ")
	}
	if err := w.notifier.Info(ctx, msg); err != nil {
		return err
	}
	if err := w.writeLastStatusReportAt(now); err != nil {
		w.log("WARN: writeLastStatusReportAt: %v", err)
	}
	_ = sig
	return nil
}

func (w *Watchdog) statusReportMarkerPath() string {
	if w == nil || w.config == nil || strings.TrimSpace(w.config.Project.Tracker) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(w.config.Project.Tracker), ".status-report-last")
}

func (w *Watchdog) readLastStatusReportAt() (time.Time, error) {
	p := w.statusReportMarkerPath()
	if p == "" {
		return time.Time{}, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}, err
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func (w *Watchdog) writeLastStatusReportAt(ts time.Time) error {
	p := w.statusReportMarkerPath()
	if p == "" {
		return nil
	}
	return os.WriteFile(p, []byte(ts.UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func (w *Watchdog) completionSignature() string {
	if w == nil || w.tracker == nil {
		return ""
	}
	ids := make([]string, 0, len(w.tracker.Tickets))
	for id := range w.tracker.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		tk := w.tracker.Tickets[id]
		b.WriteString(id)
		b.WriteString("|")
		b.WriteString(tk.Status)
		b.WriteString("|")
		b.WriteString(tk.SHA)
		b.WriteString("\n")
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

func (w *Watchdog) completionMarkerPath() string {
	if w == nil || w.config == nil || strings.TrimSpace(w.config.Project.Tracker) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(w.config.Project.Tracker), ".completion-notified")
}

func (w *Watchdog) wasCompletionNotified(signature string) bool {
	if strings.TrimSpace(signature) == "" {
		return false
	}
	p := w.completionMarkerPath()
	if p == "" {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "{") {
		var meta map[string]any
		if json.Unmarshal([]byte(raw), &meta) == nil {
			if s, ok := meta["signature"].(string); ok {
				return s == signature
			}
		}
	}
	return raw == signature
}

func (w *Watchdog) markCompletionNotified(signature string) error {
	if strings.TrimSpace(signature) == "" {
		return nil
	}
	p := w.completionMarkerPath()
	if p == "" {
		return nil
	}
	meta := map[string]any{
		"signature": signature,
		"project":   w.tracker.Project,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func (w *Watchdog) buildCompletionReport() string {
	stats := w.tracker.Stats()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🏁 *Project Complete: %s*\n\n", w.tracker.Project))
	b.WriteString(fmt.Sprintf("✅ Done: %d | ❌ Failed: %d | Total: %d\n\n", stats.Done, stats.Failed, stats.Total))

	// List all tickets with status
	ids := make([]string, 0, len(w.tracker.Tickets))
	for id := range w.tracker.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	b.WriteString("*Tickets:*\n")
	for _, id := range ids {
		tk := w.tracker.Tickets[id]
		icon := "✅"
		if tk.Status == "failed" {
			icon = "❌"
		}
		sha := ""
		if tk.SHA != "" && len(tk.SHA) >= 7 {
			sha = " (" + tk.SHA[:7] + ")"
		}
		b.WriteString(fmt.Sprintf("%s %s%s — %s\n", icon, id, sha, tk.Desc))
	}

	if stats.Failed > 0 {
		b.WriteString("\n⚠️ *Failed tickets need manual review*")
	} else {
		b.WriteString("\n*Next steps:*\n")
		b.WriteString("• Merge all branches to main\n")
		b.WriteString("• Run full test suite: `go test ./...`\n")
		b.WriteString("• Build + install: `go install`\n")
		b.WriteString("• Push to GitHub")
	}
	return b.String()
}

// SpawnTicket creates a worktree and launches an agent for ticketID.
func (w *Watchdog) SpawnTicket(ctx context.Context, ticketID string) error {
	tk, ok := w.tracker.Tickets[ticketID]
	if !ok {
		return fmt.Errorf("ticket %q not found", ticketID)
	}

	dec, _ := w.guardianCheck(ctx, guardian.EventBeforeSpawn, ticketID, tk)
	if dec.Result == guardian.ResultBlock {
		_ = w.appendEvent("guardian_block", ticketID, map[string]any{"event": "before_spawn", "rule": dec.RuleID, "reason": dec.Reason, "evidence": dec.EvidencePath})
		return fmt.Errorf("guardian blocked spawn for %s: %s", ticketID, dec.Reason)
	}
	if w != nil && w.config != nil && w.config.Project.RequireExplicitRole && strings.TrimSpace(tk.Profile) == "" {
		return fmt.Errorf("ticket %s missing explicit role/profile", ticketID)
	}

	branch := strings.TrimSpace(tk.Branch)
	if branch == "" {
		branch = "feat/" + ticketID
	}

	workDir := w.worktree.Path(ticketID)
	if !w.worktree.Exists(ticketID) {
		createdPath, err := w.worktree.Create(ticketID, branch)
		if err != nil {
			return err
		}
		if strings.TrimSpace(createdPath) != "" {
			workDir = createdPath
		}
	}

	srcPrompt := filepath.Join(w.config.Project.PromptDir, ticketID+".md")
	promptBody, err := os.ReadFile(srcPrompt)
	if err != nil {
		return fmt.Errorf("missing prompt file for %s at %s", ticketID, srcPrompt)
	}

	// Assemble layered prompt: governance → spec → profile → ticket → footer
	assembled := w.assemblePromptForTicket(ticketID, tk, promptBody)

	promptPath := filepath.Join(workDir, ".codex-prompt.md")
	if err := os.WriteFile(promptPath, assembled, 0o644); err != nil {
		return fmt.Errorf("write prompt %s: %w", promptPath, err)
	}

	spawnBackend, spawnBackendType, err := w.spawnBackendForTicket(ticketID, tk)
	if err != nil {
		return err
	}
	model := w.resolveSpawnModelForBackend(ticketID, tk, spawnBackendType)

	if w.dryRun {
		_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would spawn %s", ticketID))
		return nil
	}
	w.writeSpawnMarker(ticketID)

	handle, err := spawnBackend.Spawn(ctx, backend.SpawnConfig{
		TicketID:    ticketID,
		Branch:      branch,
		WorkDir:     workDir,
		ProjectDir:  w.projectRoot(),
		PromptFile:  promptPath,
		Model:       model,
		Effort:      w.config.Backend.Effort,
		ProjectName: w.config.Project.Name,
		SpawnFile:   w.ticketSpawnFile(ticketID),
		ExitFile:    w.ticketExitFile(ticketID),
	})
	if err != nil {
		return err
	}

	tk.Status = tracker.StatusRunning
	tk.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if tk.Branch == "" {
		tk.Branch = branch
	}
	tk.SessionName = strings.TrimSpace(handle.SessionName)
	if tk.SessionName == "" {
		tk.SessionName = w.sessionNameForTicket(ticketID)
	}
	tk.SessionBackend = spawnBackendType
	tk.SessionModel = model
	w.tracker.Tickets[ticketID] = tk

	if err := w.saveTracker(); err != nil {
		return err
	}
	return w.appendEvent("ticket_spawned", ticketID, map[string]any{
		"session": handle.SessionName,
		"backend": spawnBackendType,
		"model":   model,
	})
}

func (w *Watchdog) spawnBackendForTicket(ticketID string, tk tracker.Ticket) (backend.AgentBackend, string, error) {
	requested := w.resolveSpawnBackendType(ticketID, tk)
	be, resolvedType, err := w.backendForType(requested)
	if err == nil {
		return be, resolvedType, nil
	}
	fallbackType := w.defaultBackendType()
	if resolvedType != fallbackType {
		if fb, _, ferr := w.backendForType(fallbackType); ferr == nil {
			w.log("WARN: backend %q unavailable for %s; falling back to %q", resolvedType, ticketID, fallbackType)
			return fb, fallbackType, nil
		}
	}
	return nil, resolvedType, err
}

func (w *Watchdog) runtimeBackendForTicket(tk tracker.Ticket) (backend.AgentBackend, string, error) {
	requested := strings.TrimSpace(tk.SessionBackend)
	if requested == "" {
		requested = w.defaultBackendType()
	}
	be, resolvedType, err := w.backendForType(requested)
	if err == nil {
		return be, resolvedType, nil
	}
	fallbackType := w.defaultBackendType()
	if resolvedType != fallbackType {
		if fb, _, ferr := w.backendForType(fallbackType); ferr == nil {
			w.log("WARN: backend %q unavailable for running session; falling back to %q", resolvedType, fallbackType)
			return fb, fallbackType, nil
		}
	}
	return nil, resolvedType, err
}

func (w *Watchdog) backendForType(backendType string) (backend.AgentBackend, string, error) {
	key := normalizedBackendType(backendType)
	if w == nil {
		return nil, key, fmt.Errorf("watchdog is nil")
	}

	w.backendMu.Lock()
	defer w.backendMu.Unlock()

	if w.backendCache == nil {
		w.backendCache = map[string]backend.AgentBackend{}
	}
	if be, ok := w.backendCache[key]; ok && be != nil {
		return be, key, nil
	}

	if w.backend != nil && key == w.defaultBackendType() {
		w.backendCache[key] = w.backend
		return w.backend, key, nil
	}
	if w.backendFactory == nil {
		return nil, key, fmt.Errorf("backend factory is not configured")
	}

	be, err := w.backendFactory(key)
	if err != nil {
		return nil, key, err
	}
	w.backendCache[key] = be
	return be, key, nil
}

func (w *Watchdog) defaultBackendType() string {
	if w == nil || w.config == nil {
		return normalizedBackendType("")
	}
	return normalizedBackendType(w.config.Backend.Type)
}

func normalizedBackendType(v string) string {
	n := strings.ToLower(strings.TrimSpace(v))
	if n == "" {
		return backend.TypeCodexTmux
	}
	return n
}

func (w *Watchdog) sessionNameForTicket(ticketID string) string {
	if w != nil && w.config != nil && strings.TrimSpace(w.config.Project.Name) != "" {
		return swarmSessionPrefix + strings.TrimSpace(w.config.Project.Name) + "_" + ticketID
	}
	return swarmSessionPrefix + ticketID
}

func (w *Watchdog) sessionHandleForTicket(ticketID string, tk tracker.Ticket) backend.AgentHandle {
	sessionName := strings.TrimSpace(tk.SessionName)
	if sessionName == "" {
		sessionName = w.sessionNameForTicket(ticketID)
	}
	h := backend.AgentHandle{SessionName: sessionName}
	if ts := parseTimestamp(tk.StartedAt); !ts.IsZero() {
		h.StartedAt = ts
	}
	return h
}

// projectRoot returns the repository root directory.
func (w *Watchdog) projectRoot() string {
	if w == nil || w.config == nil {
		return ""
	}
	repo := strings.TrimSpace(w.config.Project.Repo)
	if repo != "" {
		if abs, err := filepath.Abs(repo); err == nil {
			return abs
		}
		return repo
	}
	trackerPath := strings.TrimSpace(w.config.Project.Tracker)
	if trackerPath == "" {
		return ""
	}
	return filepath.Dir(filepath.Dir(trackerPath))
}

// assemblePrompt builds the full layered prompt:
// Layer 1: AGENTS.md (governance)
// Layer 2: spec_file (project context)
// Layer 3: profile (agent role)
// Layer 4: ticket prompt (the actual task)
// Layer 5: footer (quality gates, commit rules)
func (w *Watchdog) assemblePrompt(tk tracker.Ticket, ticketPrompt []byte) []byte {
	return w.assemblePromptForTicket("", tk, ticketPrompt)
}

func (w *Watchdog) assemblePromptForTicket(ticketID string, tk tracker.Ticket, ticketPrompt []byte) []byte {
	var parts [][]byte
	root := w.projectRoot()
	profileName := w.selectProfileName(ticketID, tk)

	// Layer 1: AGENTS.md
	agentsPath := filepath.Join(root, "AGENTS.md")
	if data, err := os.ReadFile(agentsPath); err == nil {
		parts = append(parts, data)
	}

	// Layer 2: spec_file
	if w.config.Project.SpecFile != "" {
		specPath := w.config.Project.SpecFile
		if !filepath.IsAbs(specPath) {
			specPath = filepath.Join(root, specPath)
		}
		if data, err := os.ReadFile(specPath); err == nil {
			parts = append(parts, []byte("# Project Specification\n\n"))
			parts = append(parts, data)
		}
	}

	// Layer 3: profile (ticket-level overrides inferred/default profile)
	if profileName != "" {
		profilePath := filepath.Join(root, ".agents", "profiles", profileName+".md")
		if data, err := os.ReadFile(profilePath); err == nil {
			parts = append(parts, []byte("# Agent Profile\n\n"))
			parts = append(parts, data)
		} else {
			w.log("WARN: profile %q not found at %s", profileName, profilePath)
		}
	}

	// Layer 4: ticket prompt
	parts = append(parts, ticketPrompt)

	// Layer 5: footer
	parts = append(parts, loadPromptFooter(w.config.Project.PromptDir))
	if suffix := reviewOutputSuffix(profileName); len(suffix) > 0 {
		parts = append(parts, suffix)
	}

	return joinParts(parts)
}

func loadPromptFooter(promptDir string) []byte {
	footerPath := filepath.Join(filepath.Dir(promptDir), "prompt-footer.md")
	if data, err := os.ReadFile(footerPath); err == nil {
		if strings.TrimSpace(string(data)) != "" {
			return data
		}
	}
	return []byte(defaultPromptFooter)
}

func (w *Watchdog) selectProfileName(_ string, tk tracker.Ticket) string {
	return strings.TrimSpace(tk.Profile)
}

func (w *Watchdog) resolveSpawnModel(ticketID string, tk tracker.Ticket) string {
	return w.resolveSpawnModelForBackend(ticketID, tk, w.defaultBackendType())
}

func (w *Watchdog) resolveSpawnModelForBackend(ticketID string, tk tracker.Ticket, backendType string) string {
	defaultModel := ""
	if w != nil && w.config != nil {
		defaultModel = strings.TrimSpace(w.config.Backend.Model)
	}

	profileName := w.selectProfileName(ticketID, tk)
	if profileName == "" {
		return defaultModel
	}

	profileModel := w.profileFrontmatterModel(profileName)
	if profileModel == "" {
		return defaultModel
	}

	if isCodexBackendType(backendType) && !isCodexCompatibleModel(profileModel) {
		w.log("WARN: profile %q requested model %q; backend %q using fallback %q", profileName, profileModel, backendType, defaultModel)
		return defaultModel
	}

	return profileModel
}

func (w *Watchdog) resolveSpawnBackendType(ticketID string, tk tracker.Ticket) string {
	defaultType := w.defaultBackendType()
	profileName := w.selectProfileName(ticketID, tk)
	if profileName == "" {
		return defaultType
	}
	profileBackend := w.profileFrontmatterBackend(profileName)
	if profileBackend == "" {
		return defaultType
	}
	return profileBackend
}

func (w *Watchdog) profileFrontmatterModel(profileName string) string {
	root := w.projectRoot()
	if root == "" || strings.TrimSpace(profileName) == "" {
		return ""
	}
	profilePath := filepath.Join(root, ".agents", "profiles", profileName+".md")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return ""
	}
	return parseProfileFrontmatterModel(data)
}

func (w *Watchdog) profileFrontmatterBackend(profileName string) string {
	root := w.projectRoot()
	if root == "" || strings.TrimSpace(profileName) == "" {
		return ""
	}
	profilePath := filepath.Join(root, ".agents", "profiles", profileName+".md")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return ""
	}
	return parseProfileFrontmatterBackend(data)
}

func parseProfileFrontmatterModel(data []byte) string {
	return parseProfileFrontmatterValue(data, "model")
}

func parseProfileFrontmatterBackend(data []byte) string {
	v := parseProfileFrontmatterValue(data, "backend")
	if v == "" {
		return ""
	}
	return normalizedBackendType(v)
}

func parseProfileFrontmatterValue(data []byte, key string) string {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return ""
	}

	for i := 1; i < end; i++ {
		k, v, ok := strings.Cut(lines[i], ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		value := strings.TrimSpace(v)
		value = strings.Trim(value, "'\"")
		value = strings.TrimSpace(value)
		return value
	}
	return ""
}

func isCodexBackendType(backendType string) bool {
	bt := normalizedBackendType(backendType)
	return bt == backend.TypeCodexTmux
}

func isCodexCompatibleModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if strings.HasPrefix(m, "gpt-") || strings.Contains(m, "codex") {
		return true
	}
	return strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4")
}

func reviewOutputSuffix(profileName string) []byte {
	path := ""
	switch strings.TrimSpace(profileName) {
	case "code-reviewer":
		path = "swarm/features/<feature>/review-report.json"
	case "security-reviewer":
		path = "swarm/features/<feature>/sec-report.json"
	default:
		return nil
	}
	return []byte(fmt.Sprintf(`## OUTPUT FORMAT (mandatory)

You are a READ-ONLY reviewer. Do NOT modify any source files.

Output your findings as a JSON file at %q.

Use this exact schema:
{
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "security|correctness|performance|style|documentation",
      "file": "path/to/file",
      "line": 42,
      "title": "Short description",
      "description": "What is wrong and why it matters",
      "suggested_fix": "Specific action to take"
    }
  ],
  "verdict": "BLOCK|WARN|PASS",
  "summary": "N critical, N high, N medium findings"
}

Severity levels: critical, high, medium, low
Verdict: BLOCK if any critical or high. WARN if only medium/low. PASS if clean.

After writing the JSON, commit and push it.
`, path))
}

// joinParts concatenates byte slices with double-newline separators.
func joinParts(parts [][]byte) []byte {
	sep := []byte("\n\n---\n\n")
	total := 0
	for _, p := range parts {
		total += len(p) + len(sep)
	}
	buf := make([]byte, 0, total)
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = append(buf, p...)
	}
	return buf
}

func parseReviewSource(ticketID string) (feature, reportName string, ok bool) {
	if strings.HasPrefix(ticketID, "review-") && len(ticketID) > len("review-") {
		return strings.TrimPrefix(ticketID, "review-"), "review-report.json", true
	}
	if strings.HasPrefix(ticketID, "sec-") && len(ticketID) > len("sec-") {
		return strings.TrimPrefix(ticketID, "sec-"), "sec-report.json", true
	}
	return "", "", false
}

func isActionableSeverity(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high":
		return true
	default:
		return false
	}
}

func (w *Watchdog) nextFixIndex(feature string) int {
	prefix := "fix-" + feature + "-"
	next := 1
	for id := range w.tracker.Tickets {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		n, err := parseFixTicketIndex(id, prefix)
		if err != nil {
			continue
		}
		if n >= next {
			next = n + 1
		}
	}
	return next
}

func parseFixTicketIndex(ticketID, prefix string) (int, error) {
	raw := strings.TrimPrefix(ticketID, prefix)
	if raw == ticketID || raw == "" {
		return 0, fmt.Errorf("invalid fix ticket id %q", ticketID)
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid fix ticket number in %q", ticketID)
	}
	if fmt.Sprintf("%d", n) != raw {
		return 0, fmt.Errorf("invalid fix ticket suffix in %q", ticketID)
	}
	return n, nil
}

func (w *Watchdog) resolvePromptDir() string {
	promptDir := w.config.Project.PromptDir
	if filepath.IsAbs(promptDir) {
		return promptDir
	}
	return filepath.Join(w.projectRoot(), promptDir)
}

func (w *Watchdog) autoCreateFixTickets(ticketID string) (int, error) {
	if w == nil || w.tracker == nil || w.config == nil {
		return 0, nil
	}
	src, ok := w.tracker.Get(ticketID)
	if !ok {
		return 0, nil
	}
	feature, reportName, ok := parseReviewSource(ticketID)
	if !ok {
		return 0, nil
	}
	reportPath := filepath.Join(w.projectRoot(), "swarm", "features", feature, reportName)
	body, err := os.ReadFile(reportPath)
	if err != nil {
		return 0, fmt.Errorf("read findings report %s: %w", reportPath, err)
	}

	var report reviewReport
	if err := json.Unmarshal(body, &report); err != nil {
		return 0, fmt.Errorf("parse findings report %s: %w", reportPath, err)
	}

	actionable := make([]reviewFinding, 0)
	for _, finding := range report.Findings {
		if isActionableSeverity(finding.Severity) {
			actionable = append(actionable, finding)
		}
	}
	if len(actionable) == 0 {
		return 0, nil
	}

	promptDir := w.resolvePromptDir()
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir prompt dir %s: %w", promptDir, err)
	}

	next := w.nextFixIndex(feature)
	created := 0
	for _, finding := range actionable {
		fixID := fmt.Sprintf("fix-%s-%d", feature, next)
		next++
		w.tracker.Tickets[fixID] = tracker.Ticket{
			Status:  tracker.StatusTodo,
			Phase:   src.Phase,
			Depends: []string{ticketID},
			Branch:  "feat/" + fixID,
			Desc:    fixTicketDesc(finding),
			Profile: "code-agent",
		}

		promptPath := filepath.Join(promptDir, fixID+".md")
		prompt := buildFixPrompt(fixID, ticketID, finding)
		if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
			return created, fmt.Errorf("write fix prompt %s: %w", promptPath, err)
		}
		if err := w.appendEvent("fix_ticket_created", fixID, map[string]any{
			"source_ticket": ticketID,
			"severity":      strings.ToLower(strings.TrimSpace(finding.Severity)),
		}); err != nil {
			w.log("WARN: appendEvent(fix_ticket_created, %s): %v", fixID, err)
		}
		created++
	}
	return created, nil
}

func fixTicketDesc(finding reviewFinding) string {
	title := strings.TrimSpace(finding.Title)
	if title == "" {
		title = "Review finding"
	}
	return fmt.Sprintf("Fix %s finding: %s", strings.ToLower(strings.TrimSpace(finding.Severity)), title)
}

func buildFixPrompt(fixID, sourceTicket string, finding reviewFinding) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(fixID)
	b.WriteString("\n\n## Objective\n")
	b.WriteString("Fix a blocking finding from ")
	b.WriteString(sourceTicket)
	b.WriteString(".\n\n## Finding\n")
	b.WriteString("- Severity: ")
	b.WriteString(strings.ToLower(strings.TrimSpace(finding.Severity)))
	b.WriteString("\n")
	if strings.TrimSpace(finding.Category) != "" {
		b.WriteString("- Category: ")
		b.WriteString(strings.TrimSpace(finding.Category))
		b.WriteString("\n")
	}
	if strings.TrimSpace(finding.File) != "" {
		b.WriteString("- File: ")
		b.WriteString(strings.TrimSpace(finding.File))
		if finding.Line > 0 {
			b.WriteString(fmt.Sprintf(":%d", finding.Line))
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(finding.Title) != "" {
		b.WriteString("- Title: ")
		b.WriteString(strings.TrimSpace(finding.Title))
		b.WriteString("\n")
	}
	if strings.TrimSpace(finding.Description) != "" {
		b.WriteString("- Description: ")
		b.WriteString(strings.TrimSpace(finding.Description))
		b.WriteString("\n")
	}
	if strings.TrimSpace(finding.SuggestedFix) != "" {
		b.WriteString("- Suggested fix: ")
		b.WriteString(strings.TrimSpace(finding.SuggestedFix))
		b.WriteString("\n")
	}
	return b.String()
}
func (w *Watchdog) guardianCheck(ctx context.Context, ev guardian.Event, ticketID string, tk tracker.Ticket) (guardian.Decision, error) {
	if w == nil || w.guardian == nil {
		return guardian.Decision{Result: guardian.ResultAllow}, nil
	}
	dec, err := w.guardian.Evaluate(ctx, guardian.Request{
		Event:    ev,
		TicketID: ticketID,
		Phase:    tk.Phase,
		RunID:    strings.TrimSpace(tk.RunID),
		Context: map[string]any{
			"type":       tk.Type,
			"feature":    tk.Feature,
			"profile":    strings.TrimSpace(tk.Profile),
			"verify_cmd": strings.TrimSpace(tk.VerifyCmd),
		},
	})
	if err != nil {
		return guardian.Decision{Result: guardian.ResultWarn, Reason: err.Error()}, nil
	}
	if dec.Result == "" {
		dec.Result = guardian.ResultAllow
	}
	return dec, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (w *Watchdog) verifyTicket(ticketID string, tk tracker.Ticket) (bool, error) {
	verifyCmd := strings.TrimSpace(tk.VerifyCmd)
	if verifyCmd == "" && w != nil && w.config != nil {
		verifyCmd = strings.TrimSpace(w.config.Integration.VerifyCmd)
	}
	if verifyCmd == "" {
		if w != nil && w.config != nil && w.config.Project.RequireVerifyCmd {
			return false, fmt.Errorf("missing verify_cmd for ticket %s", ticketID)
		}
		w.log("WARN: ticket %s has no verify_cmd and no integration.verify_cmd; allowing completion", ticketID)
		return true, nil
	}
	wtPath := w.worktree.Path(ticketID)
	cmd := exec.Command("bash", "-lc", verifyCmd)
	cmd.Dir = wtPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			w.log("WARN: verify failed for %s: %s", ticketID, truncateText(string(out), 400))
		}
		return false, err
	}
	return true, nil
}

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(s[:max])
}

func (w *Watchdog) appendEvent(eventType, ticketID string, data map[string]any) error {
	if w.events == nil || w.dryRun {
		return nil
	}
	if data == nil {
		data = map[string]any{}
	}
	if w != nil && w.tracker != nil {
		if tk, ok := w.tracker.Get(ticketID); ok {
			if _, exists := data["phase"]; !exists {
				data["phase"] = tk.Phase
			}
			if _, exists := data["run_id"]; !exists && strings.TrimSpace(tk.RunID) != "" {
				data["run_id"] = strings.TrimSpace(tk.RunID)
			}
			if _, exists := data["role"]; !exists && strings.TrimSpace(tk.Profile) != "" {
				data["role"] = strings.TrimSpace(tk.Profile)
			}
		}
	}
	return w.events.Append(Event{Type: eventType, Ticket: ticketID, Timestamp: time.Now().UTC(), Data: data})
}

func (w *Watchdog) saveTracker() error {
	if w.dryRun || w.tracker == nil || w.config == nil || strings.TrimSpace(w.config.Project.Tracker) == "" {
		return nil
	}
	return w.tracker.SaveTo(w.config.Project.Tracker)
}

func (w *Watchdog) maxRetries() int {
	if w.config != nil && w.config.Watchdog.MaxRetries > 0 {
		return w.config.Watchdog.MaxRetries
	}
	return 2
}

func (w *Watchdog) runtimeExceeded(startedAt string) bool {
	maxRuntime := 45 * time.Minute
	if w.config != nil {
		if d, err := time.ParseDuration(strings.TrimSpace(w.config.Watchdog.MaxRuntime)); err == nil && d > 0 {
			maxRuntime = d
		}
	}
	started := parseTimestamp(startedAt)
	if started.IsZero() {
		return false
	}
	return time.Since(started) > maxRuntime
}

func (w *Watchdog) runningAgentCount(ctx context.Context) (int, error) {
	sessions, err := w.listRunningSessions(ctx)
	if err != nil {
		return 0, err
	}
	if sessions != nil {
		return len(sessions), nil
	}

	count := 0
	for _, ticketID := range runningTicketIDs(w.tracker) {
		tk := w.tracker.Tickets[ticketID]
		runningBackend, _, err := w.runtimeBackendForTicket(tk)
		if err != nil {
			return 0, err
		}
		if runningBackend.IsAlive(w.sessionHandleForTicket(ticketID, tk)) {
			count++
		}
	}
	return count, nil
}

func (w *Watchdog) listRunningSessions(ctx context.Context) ([]string, error) {
	type projectSessionLister interface {
		ListSessionsForProject(context.Context, string) ([]string, error)
	}
	type sessionLister interface {
		ListSessions(context.Context) ([]string, error)
	}

	types := map[string]struct{}{w.defaultBackendType(): {}}
	for _, ticketID := range runningTicketIDs(w.tracker) {
		tk := w.tracker.Tickets[ticketID]
		if bt := strings.TrimSpace(tk.SessionBackend); bt != "" {
			types[normalizedBackendType(bt)] = struct{}{}
		}
	}

	projectName := ""
	if w != nil && w.config != nil {
		projectName = w.config.Project.Name
	}

	usedLister := false
	set := map[string]struct{}{}
	keys := make([]string, 0, len(types))
	for k := range types {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, backendType := range keys {
		be, _, err := w.backendForType(backendType)
		if err != nil {
			return nil, err
		}
		if lister, ok := be.(projectSessionLister); ok {
			usedLister = true
			sessions, err := lister.ListSessionsForProject(ctx, projectName)
			if err != nil {
				return nil, err
			}
			for _, s := range sessions {
				if strings.TrimSpace(s) == "" {
					continue
				}
				set[s] = struct{}{}
			}
			continue
		}
		if lister, ok := be.(sessionLister); ok {
			usedLister = true
			sessions, err := lister.ListSessions(ctx)
			if err != nil {
				return nil, err
			}
			for _, s := range sessions {
				if strings.TrimSpace(s) == "" {
					continue
				}
				set[s] = struct{}{}
			}
		}
	}

	if !usedLister {
		return nil, nil
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func runningTicketIDs(tr *tracker.Tracker) []string {
	if tr == nil {
		return nil
	}
	ids := make([]string, 0)
	for id, tk := range tr.Tickets {
		if tk.Status == tracker.StatusRunning {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func parseTimestamp(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

type buildFeatureStatus struct {
	BuildIDs          []string
	BuildDone         int
	BuildTotal        int
	MaxBuildPhase     int
	PostBuildStepsSet map[string]bool
}

func (w *Watchdog) ensurePostBuildTickets(ctx context.Context) error {
	order, parallelGroups, enabled := w.postBuildPlan()
	if !enabled || w.tracker == nil {
		return nil
	}

	features := w.collectBuildFeatureStatus(order)
	if len(features) == 0 {
		return nil
	}

	featureNames := make([]string, 0, len(features))
	for feature, fs := range features {
		if fs.BuildTotal == 0 || fs.BuildDone != fs.BuildTotal {
			continue
		}
		missing := false
		for _, step := range order {
			if !fs.PostBuildStepsSet[step] {
				missing = true
				break
			}
		}
		if missing {
			featureNames = append(featureNames, feature)
		}
	}
	if len(featureNames) == 0 {
		return nil
	}
	sort.Strings(featureNames)

	if w.dryRun {
		for _, feature := range featureNames {
			_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would create post-build tickets for feature %s", feature))
		}
		return nil
	}

	changed := false
	for _, feature := range featureNames {
		fs := features[feature]
		created := w.createPostBuildTicketsForFeature(feature, fs, order, parallelGroups)
		if created == 0 {
			continue
		}
		changed = true
		msg := fmt.Sprintf("feature %s build complete — created %d post-build tickets", feature, created)
		_ = w.notifier.Info(ctx, msg)
		if err := w.appendEvent("post_build_generated", "", map[string]any{
			"feature": feature,
			"created": created,
		}); err != nil {
			w.log("WARN: appendEvent(post_build_generated, %s): %v", feature, err)
		}
	}

	if changed {
		return w.saveTracker()
	}
	return nil
}

func (w *Watchdog) postBuildPlan() ([]string, [][]string, bool) {
	if w == nil || w.config == nil {
		return nil, nil, false
	}
	order := normalizeStepList(w.config.PostBuild.Order)
	if len(order) == 0 {
		return nil, nil, false
	}
	parallelGroups := make([][]string, 0, len(w.config.PostBuild.ParallelGroups))
	for _, group := range w.config.PostBuild.ParallelGroups {
		steps := normalizeStepList(group)
		if len(steps) >= 2 {
			parallelGroups = append(parallelGroups, steps)
		}
	}
	return order, parallelGroups, true
}

func normalizeStepList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		step := strings.TrimSpace(raw)
		if step == "" {
			continue
		}
		if _, ok := seen[step]; ok {
			continue
		}
		seen[step] = struct{}{}
		out = append(out, step)
	}
	return out
}

func (w *Watchdog) collectBuildFeatureStatus(order []string) map[string]*buildFeatureStatus {
	stepSet := make(map[string]struct{}, len(order))
	for _, step := range order {
		stepSet[step] = struct{}{}
	}

	features := map[string]*buildFeatureStatus{}
	for id, tk := range w.tracker.Tickets {
		if feature, ok := buildFeatureFromTicket(id, tk, stepSet); ok {
			fs := features[feature]
			if fs == nil {
				fs = &buildFeatureStatus{PostBuildStepsSet: map[string]bool{}}
				features[feature] = fs
			}
			fs.BuildTotal++
			if tk.Status == tracker.StatusDone {
				fs.BuildDone++
			}
			fs.BuildIDs = append(fs.BuildIDs, id)
			if tk.Phase > fs.MaxBuildPhase {
				fs.MaxBuildPhase = tk.Phase
			}
			continue
		}
		if step, feature, ok := postBuildStepFromTicket(id, tk, stepSet); ok {
			fs := features[feature]
			if fs == nil {
				fs = &buildFeatureStatus{PostBuildStepsSet: map[string]bool{}}
				features[feature] = fs
			}
			fs.PostBuildStepsSet[step] = true
		}
	}

	for _, fs := range features {
		sort.Strings(fs.BuildIDs)
	}
	return features
}

func buildFeatureFromTicket(id string, tk tracker.Ticket, postBuildSteps map[string]struct{}) (string, bool) {
	ticketType := strings.TrimSpace(tk.Type)
	if ticketType != "" {
		if _, isPostBuild := postBuildSteps[ticketType]; isPostBuild {
			return "", false
		}
		if ticketType == "build" {
			feature := strings.TrimSpace(tk.Feature)
			if feature == "" {
				feature, _ = parseBuildID(id)
			}
			if feature != "" {
				return feature, true
			}
		}
		return "", false
	}
	feature, ok := parseBuildID(id)
	if !ok || feature == "" {
		return "", false
	}
	return feature, true
}

func parseBuildID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	i := strings.LastIndex(id, "-")
	if i <= 0 || i+1 >= len(id) {
		return "", false
	}
	feature := strings.TrimSpace(id[:i])
	suffix := strings.TrimSpace(id[i+1:])
	if feature == "" || suffix == "" {
		return "", false
	}
	if _, err := strconv.Atoi(suffix); err != nil {
		return "", false
	}
	return feature, true
}

func postBuildStepFromTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) (step, feature string, ok bool) {
	ticketType := strings.TrimSpace(tk.Type)
	feature = strings.TrimSpace(tk.Feature)
	if ticketType != "" {
		if _, isStep := stepSet[ticketType]; isStep && feature != "" {
			return ticketType, feature, true
		}
	}
	i := strings.Index(id, "-")
	if i <= 0 || i+1 >= len(id) {
		return "", "", false
	}
	step = strings.TrimSpace(id[:i])
	feature = strings.TrimSpace(id[i+1:])
	if step == "" || feature == "" {
		return "", "", false
	}
	if _, isStep := stepSet[step]; !isStep {
		return "", "", false
	}
	return step, feature, true
}

func (w *Watchdog) createPostBuildTicketsForFeature(
	feature string,
	fs *buildFeatureStatus,
	order []string,
	parallelGroups [][]string,
) int {
	if fs == nil {
		return 0
	}
	phase := fs.MaxBuildPhase
	if phase <= 0 {
		phase = 1
	}

	stages := buildPostBuildStages(order, parallelGroups)
	if len(stages) == 0 {
		return 0
	}

	created := 0
	prevStageIDs := append([]string(nil), fs.BuildIDs...)
	sort.Strings(prevStageIDs)

	for _, stage := range stages {
		nextStageIDs := make([]string, 0, len(stage))
		for _, step := range stage {
			id := step + "-" + feature
			nextStageIDs = append(nextStageIDs, id)
			if _, exists := w.tracker.Tickets[id]; exists {
				fs.PostBuildStepsSet[step] = true
				continue
			}
			tk := tracker.Ticket{
				Status:  tracker.StatusTodo,
				Phase:   phase,
				Depends: append([]string(nil), prevStageIDs...),
				Type:    step,
				Feature: feature,
				Branch:  "feat/" + id,
				Desc:    postBuildDescription(step, feature),
				Profile: postBuildProfile(step),
			}
			if len(tk.Depends) == 0 {
				tk.Depends = []string{}
			}
			w.tracker.Tickets[id] = tk
			promptDir := w.resolvePromptDir()
			if err := os.MkdirAll(promptDir, 0o755); err == nil {
				promptPath := filepath.Join(promptDir, id+".md")
				if _, statErr := os.Stat(promptPath); os.IsNotExist(statErr) {
					_ = os.WriteFile(promptPath, []byte(buildPostBuildPrompt(id, step, feature, tk.Depends)), 0o644)
				}
			}
			fs.PostBuildStepsSet[step] = true
			created++
		}
		sort.Strings(nextStageIDs)
		prevStageIDs = nextStageIDs
	}
	return created
}

func buildPostBuildStages(order []string, parallelGroups [][]string) [][]string {
	order = normalizeStepList(order)
	if len(order) == 0 {
		return nil
	}
	n := len(order)
	indexByStep := make(map[string]int, n)
	for i, step := range order {
		indexByStep[step] = i
	}

	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	grouped := make([]bool, n)
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra := find(a)
		rb := find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}

	for _, group := range parallelGroups {
		steps := normalizeStepList(group)
		if len(steps) < 2 {
			continue
		}
		indices := make([]int, 0, len(steps))
		for _, step := range steps {
			if idx, ok := indexByStep[step]; ok {
				indices = append(indices, idx)
			}
		}
		if len(indices) < 2 {
			continue
		}
		sort.Ints(indices)
		contiguous := true
		for i := 1; i < len(indices); i++ {
			if indices[i] != indices[i-1]+1 {
				contiguous = false
				break
			}
		}
		if !contiguous {
			continue
		}
		overlaps := false
		for _, idx := range indices {
			if grouped[idx] {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		base := indices[0]
		for _, idx := range indices[1:] {
			union(base, idx)
		}
		for _, idx := range indices {
			grouped[idx] = true
		}
	}

	stageMap := map[int][]int{}
	for i := 0; i < n; i++ {
		root := find(i)
		stageMap[root] = append(stageMap[root], i)
	}

	roots := make([]int, 0, len(stageMap))
	for root := range stageMap {
		roots = append(roots, root)
	}
	sort.Ints(roots)

	stages := make([][]string, 0, len(roots))
	for _, root := range roots {
		indices := stageMap[root]
		sort.Ints(indices)
		stage := make([]string, 0, len(indices))
		for _, idx := range indices {
			stage = append(stage, order[idx])
		}
		stages = append(stages, stage)
	}
	return stages
}

func buildPostBuildPrompt(ticketID, step, feature string, depends []string) string {
	depText := "none"
	if len(depends) > 0 {
		depText = strings.Join(depends, ", ")
	}
	return fmt.Sprintf(`# %s

## Objective
Run post-build step %q for feature %q.

## Dependencies
%s

## Scope
- Execute the %s workflow for feature %s
- Produce concrete outputs and commit them
- Do not modify unrelated features

## Verify
Follow the project verify command and include evidence in commit message.
`, ticketID, step, feature, depText, step, feature)
}

func postBuildProfile(step string) string {
	switch strings.TrimSpace(step) {
	case "gap", "review", "mem":
		return "code-reviewer"
	case "tst":
		return "e2e-runner"
	case "sec":
		return "security-reviewer"
	case "doc":
		return "doc-updater"
	case "clean":
		return "refactor-cleaner"
	default:
		return ""
	}
}

func postBuildDescription(step, feature string) string {
	switch strings.TrimSpace(step) {
	case "int":
		return fmt.Sprintf("Integration merge for %s", feature)
	case "gap":
		return fmt.Sprintf("Gap assessment for %s", feature)
	case "tst":
		return fmt.Sprintf("E2E + build verification for %s", feature)
	case "review":
		return fmt.Sprintf("Code review for %s", feature)
	case "sec":
		return fmt.Sprintf("Security review for %s", feature)
	case "doc":
		return fmt.Sprintf("Documentation update for %s", feature)
	case "clean":
		return fmt.Sprintf("Refactor/cleanup for %s", feature)
	case "mem":
		return fmt.Sprintf("Lessons learned for %s", feature)
	default:
		return fmt.Sprintf("Post-build %s for %s", step, feature)
	}
}
