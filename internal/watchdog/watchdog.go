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
		handle := w.sessionHandleForTicket(ticketID, tk)
		runningBackend, _, err := w.runtimeBackendForTicket(tk)
		if err != nil {
			w.log("WARN: backend for %s: %v", ticketID, err)
			continue
		}

		if runningBackend.HasExited(handle) {
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

		if runningBackend.IsAlive(handle) && w.runtimeExceeded(tk.StartedAt) && !w.stuckAlerts[ticketID] {
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
		// Auto-generate prompt from ticket description + project spec
		tk, _ := w.tracker.Get(ticketID)
		desc := tk.Desc
		if desc == "" {
			desc = "Implement " + ticketID
		}
		promptBody = []byte(fmt.Sprintf("# %s\n\n## Objective\n%s\n", ticketID, desc))
		w.log("WARN: no prompt file for %s — auto-generated from description + spec", ticketID)
		// Also save it for future reference
		_ = os.MkdirAll(filepath.Dir(srcPrompt), 0o755)
		_ = os.WriteFile(srcPrompt, promptBody, 0o644)
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

	handle, err := spawnBackend.Spawn(ctx, backend.SpawnConfig{
		TicketID:    ticketID,
		Branch:      branch,
		WorkDir:     workDir,
		PromptFile:  promptPath,
		Model:       model,
		Effort:      w.config.Backend.Effort,
		ProjectName: w.config.Project.Name,
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

// projectRoot returns the project root directory (parent of swarm/).
func (w *Watchdog) projectRoot() string {
	if w == nil || w.config == nil {
		return ""
	}
	trackerPath := strings.TrimSpace(w.config.Project.Tracker)
	if trackerPath == "" {
		return ""
	}
	// Tracker is at swarm/tracker.json — go up two levels
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
	profileName := w.selectProfileName(ticketID, tk)
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
	footerPath := filepath.Join(filepath.Dir(w.config.Project.PromptDir), "prompt-footer.md")
	if data, err := os.ReadFile(footerPath); err == nil {
		parts = append(parts, data)
	}

	return joinParts(parts)
}

func (w *Watchdog) selectProfileName(ticketID string, tk tracker.Ticket) string {
	if profile := strings.TrimSpace(tk.Profile); profile != "" {
		return profile
	}
	if inferred := inferProfileFromTicketID(ticketID); inferred != "" {
		return inferred
	}
	if w == nil || w.config == nil {
		return ""
	}
	return strings.TrimSpace(w.config.Project.DefaultProfile)
}

func inferProfileFromTicketID(ticketID string) string {
	id := strings.ToLower(strings.TrimSpace(ticketID))
	switch {
	case strings.HasPrefix(id, "arch-"), strings.HasPrefix(id, "arc-"):
		return "architect"
	case strings.HasPrefix(id, "gap-"), strings.HasPrefix(id, "review-"), strings.HasPrefix(id, "rev-"), strings.HasPrefix(id, "mem-"):
		return "code-reviewer"
	case strings.HasPrefix(id, "sec-"):
		return "security-reviewer"
	case strings.HasPrefix(id, "tst-"):
		return "e2e-runner"
	case strings.HasPrefix(id, "doc-"):
		return "doc-updater"
	case strings.HasPrefix(id, "clean-"), strings.HasPrefix(id, "cln-"):
		return "refactor-cleaner"
	case strings.HasPrefix(id, "fix-"):
		return "code-agent"
	default:
		return ""
	}
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
