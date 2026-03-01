package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/sysinfo"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type WatchdogStatus struct {
	Running       bool      `json:"running"`
	LastRun       time.Time `json:"last_run"`
	NextRun       time.Time `json:"next_run"`
	AlertsPending int       `json:"alerts_pending"`
}

type Watchdog interface {
	Start(context.Context)
	Stop(context.Context) error
	RunOnce(context.Context) error
	Log(lines int) []string
	Status() WatchdogStatus
}

type MemoryWatchdog struct {
	interval time.Duration
	now      func() time.Time

	mu      sync.RWMutex
	running bool
	lastRun time.Time
	nextRun time.Time
	logs    []string

	cancel context.CancelFunc
	done   chan struct{}
}

func NewMemoryWatchdog(interval time.Duration) *MemoryWatchdog {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &MemoryWatchdog{interval: interval, now: time.Now, done: make(chan struct{})}
}

func (w *MemoryWatchdog) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.running = true
	w.nextRun = w.now().Add(w.interval)
	w.mu.Unlock()

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		defer close(w.done)
		for {
			select {
			case <-loopCtx.Done():
				w.mu.Lock()
				w.running = false
				w.mu.Unlock()
				return
			case <-ticker.C:
				_ = w.RunOnce(loopCtx)
			}
		}
	}()
}

func (w *MemoryWatchdog) Stop(ctx context.Context) error {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *MemoryWatchdog) RunOnce(_ context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	w.lastRun = now
	w.nextRun = now.Add(w.interval)
	w.logs = append(w.logs, fmt.Sprintf("watchdog pass at %s", now.Format(time.RFC3339)))
	if len(w.logs) > 200 {
		w.logs = w.logs[len(w.logs)-200:]
	}
	return nil
}

func (w *MemoryWatchdog) Log(lines int) []string {
	if lines <= 0 {
		lines = 50
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if lines >= len(w.logs) {
		out := make([]string, len(w.logs))
		copy(out, w.logs)
		return out
	}
	start := len(w.logs) - lines
	out := make([]string, lines)
	copy(out, w.logs[start:])
	return out
}

func (w *MemoryWatchdog) Status() WatchdogStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return WatchdogStatus{
		Running:       w.running,
		LastRun:       w.lastRun,
		NextRun:       w.nextRun,
		AlertsPending: 0,
	}
}

type Server struct {
	cfg        *config.Config
	tracker    *tracker.Tracker
	dispatcher *dispatcher.Dispatcher
	backend    backend.AgentBackend
	watchdog   Watchdog
	events     *EventBus
	logger     *log.Logger
	startedAt  time.Time

	trackerPath string
	mu          sync.RWMutex
}

func New(
	cfg *config.Config,
	tr *tracker.Tracker,
	d *dispatcher.Dispatcher,
	be backend.AgentBackend,
	wd Watchdog,
	logger *log.Logger,
) *Server {
	if cfg == nil {
		cfg = config.Default()
	}
	if tr == nil {
		tr = tracker.New(cfg.Project.Name, map[string]tracker.Ticket{})
	}
	if d == nil {
		d = dispatcher.New(cfg, tr)
	}
	if wd == nil {
		wd = NewMemoryWatchdog(5 * time.Minute)
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Server{
		cfg:         cfg,
		tracker:     tr,
		dispatcher:  d,
		backend:     be,
		watchdog:    wd,
		events:      NewEventBus(32),
		logger:      logger,
		startedAt:   time.Now(),
		trackerPath: cfg.Project.Tracker,
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/projects", s.handleProjects)
	mux.HandleFunc("GET /api/projects/{name}/status", s.handleProjectStatus)
	mux.HandleFunc("GET /api/projects/{name}/tickets", s.handleProjectTickets)
	mux.HandleFunc("GET /api/projects/{name}/stats", s.handleProjectStats)

	mux.HandleFunc("GET /api/projects/{name}/tickets/{id}", s.handleTicket)
	mux.HandleFunc("GET /api/projects/{name}/tickets/{id}/output", s.handleTicketOutput)
	mux.HandleFunc("POST /api/projects/{name}/tickets/{id}/kill", s.handleTicketKill)
	mux.HandleFunc("POST /api/projects/{name}/tickets/{id}/respawn", s.handleTicketRespawn)
	mux.HandleFunc("POST /api/projects/{name}/tickets/{id}/done", s.handleTicketDone)
	mux.HandleFunc("POST /api/projects/{name}/tickets/{id}/fail", s.handleTicketFail)

	mux.HandleFunc("GET /api/projects/{name}/phase-gate", s.handlePhaseGate)
	mux.HandleFunc("POST /api/projects/{name}/phase-gate/approve", s.handlePhaseGateApprove)

	mux.HandleFunc("GET /api/watchdog/status", s.handleWatchdogStatus)
	mux.HandleFunc("GET /api/watchdog/log", s.handleWatchdogLog)
	mux.HandleFunc("POST /api/watchdog/run", s.handleWatchdogRun)

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	return Chain(
		mux,
		RequestLoggingMiddleware(s.logger),
		BearerAuthMiddleware(s.cfg.Serve.AuthToken),
		CORSMiddleware(s.cfg.Serve.CORS),
	)
}

func (s *Server) Events() *EventBus {
	return s.events
}

func (s *Server) Close(ctx context.Context) error {
	s.events.Close()
	if s.watchdog != nil {
		return s.watchdog.Stop(ctx)
	}
	return nil
}

func (s *Server) handleProjects(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	stats := s.tracker.Stats()
	phase := s.dispatcher.CurrentPhase()
	running := s.tracker.RunningCount()
	project := s.cfg.Project.Name
	s.mu.RUnlock()

	progress := 0
	if stats.Total > 0 {
		progress = (stats.Done * 100) / stats.Total
	}

	writeJSON(w, http.StatusOK, []map[string]any{{
		"name":           project,
		"progress":       progress,
		"phase":          phase,
		"agents_running": running,
	}})
}

func (s *Server) handleProjectStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sig, spawnable := s.dispatcher.Evaluate()
	writeJSON(w, http.StatusOK, map[string]any{
		"project":      s.tracker.Project,
		"signal":       sig,
		"spawnable":    spawnable,
		"phase_status": s.dispatcher.PhaseStatus(),
		"stats":        s.tracker.Stats(),
		"tickets":      s.tracker.Tickets,
	})
}

func (s *Server) handleProjectTickets(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]map[string]any, 0, len(s.tracker.Tickets))
	for id, tk := range s.tracker.Tickets {
		out = append(out, map[string]any{
			"id":       id,
			"status":   tk.Status,
			"progress": ticketProgress(tk),
			"desc":     tk.Desc,
			"deps":     tk.Depends,
			"phase":    tk.Phase,
			"sha":      tk.SHA,
			"runtime":  ticketRuntime(tk),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleProjectStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	s.mu.RLock()
	stats := s.tracker.Stats()
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"done":        stats.Done,
		"running":     stats.Running,
		"todo":        stats.Todo,
		"failed":      stats.Failed,
		"blocked":     stats.Blocked,
		"total":       stats.Total,
		"eta_minutes": 0,
	})
}

func (s *Server) handleTicket(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	id := r.PathValue("id")
	s.mu.RLock()
	tk, ok := s.tracker.Get(id)
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       id,
		"ticket":   tk,
		"progress": ticketProgress(tk),
	})
}

func (s *Server) handleTicketOutput(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	if s.backend == nil {
		writeError(w, http.StatusServiceUnavailable, "backend unavailable")
		return
	}
	id := r.PathValue("id")
	s.mu.RLock()
	_, ok := s.tracker.Get(id)
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	handle := backend.AgentHandle{SessionName: sessionNameForTicket(id)}
	s.writeOutputEvent(r.Context(), w, flusher, id, handle)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.writeOutputEvent(r.Context(), w, flusher, id, handle)
		}
	}
}

func (s *Server) writeOutputEvent(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, ticketID string, handle backend.AgentHandle) {
	output, err := s.backend.GetOutput(handle, 200)
	payload := map[string]any{"ticket": ticketID, "output": output}
	if err != nil {
		payload = map[string]any{"ticket": ticketID, "error": err.Error()}
	}
	writeSSE(w, flusher, "output", payload)
	select {
	case <-ctx.Done():
		return
	default:
	}
}

func (s *Server) handleTicketKill(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	if s.backend == nil {
		writeError(w, http.StatusServiceUnavailable, "backend unavailable")
		return
	}
	id := r.PathValue("id")
	if err := s.backend.Kill(backend.AgentHandle{SessionName: sessionNameForTicket(id)}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	tk, ok := s.tracker.Get(id)
	if ok {
		tk.Status = tracker.StatusTodo
		s.tracker.Tickets[id] = tk
		s.saveTrackerLocked()
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTicketRespawn(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	if s.backend == nil {
		writeError(w, http.StatusServiceUnavailable, "backend unavailable")
		return
	}
	id := r.PathValue("id")
	s.mu.Lock()
	tk, ok := s.tracker.Get(id)
	if !ok {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	cfg := backend.SpawnConfig{
		TicketID:   id,
		Branch:     tk.Branch,
		WorkDir:    s.cfg.Project.Repo,
		PromptFile: filepath.Join(s.cfg.Project.PromptDir, id+".md"),
		Model:      s.cfg.Backend.Model,
		Effort:     s.cfg.Backend.Effort,
	}
	_, err := s.backend.Spawn(r.Context(), cfg)
	if err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tk.Status = tracker.StatusRunning
	tk.StartedAt = time.Now().UTC().Format(time.RFC3339)
	s.tracker.Tickets[id] = tk
	s.saveTrackerLocked()
	s.mu.Unlock()

	s.events.Publish(EventTicketSpawned, map[string]any{
		"project": s.cfg.Project.Name,
		"ticket":  id,
		"agent":   sessionNameForTicket(id),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTicketDone(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	id := r.PathValue("id")

	var req struct {
		SHA string `json:"sha"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}

	s.mu.Lock()
	sig, spawnable := s.dispatcher.MarkDone(id, strings.TrimSpace(req.SHA))
	s.saveTrackerLocked()
	s.mu.Unlock()

	s.events.Publish(EventTicketDone, map[string]any{
		"project":        s.cfg.Project.Name,
		"ticket":         id,
		"sha":            strings.TrimSpace(req.SHA),
		"next_spawnable": spawnable,
	})
	writeJSON(w, http.StatusOK, map[string]any{"signal": sig, "spawnable": spawnable})
}

func (s *Server) handleTicketFail(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	id := r.PathValue("id")
	s.mu.Lock()
	err := s.dispatcher.MarkFailed(id)
	s.saveTrackerLocked()
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.events.Publish(EventFailure, map[string]any{
		"project": s.cfg.Project.Name,
		"ticket":  id,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePhaseGate(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	s.mu.RLock()
	ps := s.dispatcher.PhaseStatus()
	sig, _ := s.dispatcher.Evaluate()
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"phase":        ps.Phase,
		"gate_reached": ps.GateReached,
		"status":       ps,
		"signal":       sig,
	})
}

func (s *Server) handlePhaseGateApprove(w http.ResponseWriter, r *http.Request) {
	if !s.requireProject(w, r) {
		return
	}
	s.mu.Lock()
	sig, spawnable := s.dispatcher.ApprovePhaseGate()
	s.mu.Unlock()
	s.events.Publish(EventPhaseGate, map[string]any{
		"project": s.cfg.Project.Name,
		"phase":   s.dispatcher.CurrentPhase(),
		"message": "Phase gate approved",
	})
	writeJSON(w, http.StatusOK, map[string]any{"signal": sig, "spawnable": spawnable})
}

func (s *Server) handleWatchdogStatus(w http.ResponseWriter, _ *http.Request) {
	if s.watchdog == nil {
		writeJSON(w, http.StatusOK, WatchdogStatus{})
		return
	}
	writeJSON(w, http.StatusOK, s.watchdog.Status())
}

func (s *Server) handleWatchdogLog(w http.ResponseWriter, r *http.Request) {
	if s.watchdog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"lines": []string{}})
		return
	}
	lines := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("lines")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			lines = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": s.watchdog.Log(lines)})
}

func (s *Server) handleWatchdogRun(w http.ResponseWriter, r *http.Request) {
	if s.watchdog == nil {
		writeError(w, http.StatusServiceUnavailable, "watchdog unavailable")
		return
	}
	if err := s.watchdog.RunOnce(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ramMB, _ := sysinfo.AvailableRAM()
	s.mu.RLock()
	agents := s.tracker.RunningCount()
	uptime := int(time.Since(s.startedAt).Seconds())
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"ram_mb":         ramMB,
		"agents_running": agents,
		"uptime":         uptime,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id, ch := s.events.Subscribe()
	defer s.events.Unsubscribe(id)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", string(ev.Data)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", eventType)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
	flusher.Flush()
}

func (s *Server) requireProject(w http.ResponseWriter, r *http.Request) bool {
	name := r.PathValue("name")
	expected := s.cfg.Project.Name
	if expected == "" {
		expected = s.tracker.Project
	}
	if name == expected {
		return true
	}
	writeError(w, http.StatusNotFound, "project not found")
	return false
}

func ticketProgress(tk tracker.Ticket) int {
	switch tk.Status {
	case tracker.StatusDone:
		return 100
	case tracker.StatusRunning:
		return 50
	default:
		return 0
	}
}

func ticketRuntime(tk tracker.Ticket) string {
	startedAt := strings.TrimSpace(tk.StartedAt)
	if startedAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return ""
	}
	return time.Since(t).Round(time.Second).String()
}

func sessionNameForTicket(ticketID string) string {
	return "swarm-" + ticketID
}

func (s *Server) saveTrackerLocked() {
	if strings.TrimSpace(s.trackerPath) == "" {
		return
	}
	_ = s.tracker.SaveTo(s.trackerPath)
}
