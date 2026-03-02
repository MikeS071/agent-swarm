package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/notify"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

const swarmSessionPrefix = "swarm-"

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

	dryRun      bool
	retries     map[string]int
	spawnErrors map[string]int
	stuckAlerts map[string]bool
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
	return &Watchdog{
		config:      cfg,
		tracker:     tr,
		dispatcher:  d,
		backend:     be,
		worktree:    wt,
		notifier:    n,
		events:      NewEventLog(eventsPath),
		retries:     map[string]int{},
		spawnErrors: map[string]int{},
		stuckAlerts: map[string]bool{},
	}
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

// Run executes the watchdog loop at configured interval until ctx cancellation.
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

	// Re-read tracker from disk to pick up external changes (e.g. TUI phase gate approval).
	if w.tracker != nil && w.config != nil && w.config.Project.Tracker != "" {
		if fresh, err := tracker.Load(w.config.Project.Tracker); err == nil {
			// Preserve in-memory state that disk may not have (running sessions etc are already in tickets).
			// But always adopt the disk unlocked_phase if higher (TUI may have approved a gate).
			if fresh.UnlockedPhase > w.tracker.UnlockedPhase {
				w.tracker.UnlockedPhase = fresh.UnlockedPhase
				w.dispatcher.SetUnlockedPhase(fresh.UnlockedPhase)
			}
		}
	}

	if _, err := w.listRunningSessions(ctx); err != nil {
		w.log("WARN: listRunningSessions: %v", err)
	}

	w.log("pass: %d running, checking exits", len(runningTicketIDs(w.tracker)))
	for _, ticketID := range runningTicketIDs(w.tracker) {
		tk := w.tracker.Tickets[ticketID]
		handle := backend.AgentHandle{SessionName: swarmSessionPrefix + ticketID}
		if ts := parseTimestamp(tk.StartedAt); !ts.IsZero() {
			handle.StartedAt = ts
		}

		if w.backend.HasExited(handle) {
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
				sig, spawnable := w.dispatcher.MarkDone(ticketID, sha)
				delete(w.retries, ticketID)
				// Clean up agent log on successful completion
				if w.config != nil && w.config.Project.Tracker != "" {
					logFile := filepath.Join(filepath.Dir(w.config.Project.Tracker), "logs", ticketID+".log")
					os.Remove(logFile)
				}
				if err := w.appendEvent("ticket_done", ticketID, map[string]any{"sha": sha}); err != nil {
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
				if err := w.appendEvent("respawn", ticketID, map[string]any{"attempt": attempt}); err != nil {
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
			if err := w.appendEvent("ticket_failed", ticketID, map[string]any{"attempt": attempt}); err != nil {
				return err
			}
			_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s failed after %d attempts", ticketID, attempt))
			continue
		}

		if w.backend.IsAlive(handle) && w.runtimeExceeded(tk.StartedAt) && !w.stuckAlerts[ticketID] {
			w.stuckAlerts[ticketID] = true
			_ = w.notifier.Alert(ctx, fmt.Sprintf("ticket %s may be stuck (exceeded max_runtime)", ticketID))
		}
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
		w.completionSent = true
		report := w.buildCompletionReport()
		if err := w.appendEvent("project_complete", "", nil); err != nil {
			w.log("WARN: appendEvent(project_complete): %v", err)
		}
		_ = w.notifier.Alert(ctx, report)
	}

	return nil
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

	branch := strings.TrimSpace(tk.Branch)
	if branch == "" {
		branch = "feat/" + ticketID
	}

	if !w.worktree.Exists(ticketID) {
		if _, err := w.worktree.Create(ticketID, branch); err != nil {
			return err
		}
	}
	workDir := w.worktree.Path(ticketID)

	srcPrompt := filepath.Join(w.config.Project.PromptDir, ticketID+".md")
	promptBody, err := os.ReadFile(srcPrompt)
	if err != nil {
		return fmt.Errorf("read prompt %s: %w", srcPrompt, err)
	}

	promptPath := filepath.Join(workDir, ".codex-prompt.md")
	if err := os.WriteFile(promptPath, promptBody, 0o644); err != nil {
		return fmt.Errorf("write prompt %s: %w", promptPath, err)
	}

	footerPath := filepath.Join(filepath.Dir(w.config.Project.PromptDir), "prompt-footer.md")
	if footer, err := os.ReadFile(footerPath); err == nil {
		f, err := os.OpenFile(promptPath, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("\n\n")); err != nil {
			_ = f.Close()
			return err
		}
		if _, err := f.Write(footer); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	if w.dryRun {
		_ = w.notifier.Info(ctx, fmt.Sprintf("[dry-run] would spawn %s", ticketID))
		return nil
	}

	handle, err := w.backend.Spawn(ctx, backend.SpawnConfig{
		TicketID:   ticketID,
		Branch:     branch,
		WorkDir:    workDir,
		PromptFile: promptPath,
		Model:      w.config.Backend.Model,
		Effort:     w.config.Backend.Effort,
	})
	if err != nil {
		return err
	}

	tk.Status = tracker.StatusRunning
	tk.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if tk.Branch == "" {
		tk.Branch = branch
	}
	w.tracker.Tickets[ticketID] = tk

	if err := w.saveTracker(); err != nil {
		return err
	}
	return w.appendEvent("ticket_spawned", ticketID, map[string]any{"session": handle.SessionName})
}

func (w *Watchdog) appendEvent(eventType, ticketID string, data map[string]any) error {
	if w.events == nil || w.dryRun {
		return nil
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
	w.log("pass: %d running, checking exits", len(runningTicketIDs(w.tracker)))
	for _, ticketID := range runningTicketIDs(w.tracker) {
		if w.backend.IsAlive(backend.AgentHandle{SessionName: swarmSessionPrefix + ticketID}) {
			count++
		}
	}
	return count, nil
}

func (w *Watchdog) listRunningSessions(ctx context.Context) ([]string, error) {
	type sessionLister interface {
		ListSessions(context.Context) ([]string, error)
	}
	if lister, ok := w.backend.(sessionLister); ok {
		return lister.ListSessions(ctx)
	}
	return nil, nil
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
