package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type fakeBackend struct {
	killed  []string
	output  string
	spawned []backend.SpawnConfig
}

func (f *fakeBackend) Spawn(_ context.Context, cfg backend.SpawnConfig) (backend.AgentHandle, error) {
	f.spawned = append(f.spawned, cfg)
	return backend.AgentHandle{SessionName: "swarm-" + cfg.TicketID, StartedAt: time.Now()}, nil
}

func (f *fakeBackend) IsAlive(handle backend.AgentHandle) bool   { return handle.SessionName != "" }
func (f *fakeBackend) HasExited(handle backend.AgentHandle) bool { return false }
func (f *fakeBackend) GetOutput(handle backend.AgentHandle, lines int) (string, error) {
	return f.output, nil
}
func (f *fakeBackend) Kill(handle backend.AgentHandle) error {
	f.killed = append(f.killed, handle.SessionName)
	return nil
}
func (f *fakeBackend) Name() string { return "fake" }

type fakeWatchdog struct{}

func (f *fakeWatchdog) Start(context.Context)         {}
func (f *fakeWatchdog) Stop(context.Context) error    { return nil }
func (f *fakeWatchdog) RunOnce(context.Context) error { return nil }
func (f *fakeWatchdog) Log(lines int) []string        { return []string{"watchdog ok"} }
func (f *fakeWatchdog) Status() WatchdogStatus {
	return WatchdogStatus{Running: true, AlertsPending: 1}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	tr := tracker.New("test", map[string]tracker.Ticket{
		"sw-01": {Status: tracker.StatusDone, Phase: 1, Desc: "done"},
		"sw-07": {Status: tracker.StatusRunning, Phase: 2, Desc: "serve", Branch: "feat/sw-07"},
		"sw-09": {Status: tracker.StatusTodo, Phase: 2, Depends: []string{"sw-07"}, Desc: "next"},
	})
	cfg := config.Default()
	cfg.Project.Name = "test"
	cfg.Project.Repo = "."
	cfg.Project.PromptDir = "swarm/prompts"
	cfg.Project.Tracker = ""
	cfg.Backend.Model = "gpt-5.3-codex"
	cfg.Backend.Effort = "high"
	cfg.Serve.CORS = []string{"*"}
	cfg.Serve.AuthToken = ""

	d := dispatcher.New(cfg, tr)
	b := &fakeBackend{output: "PROGRESS: 1/3\nline2"}
	return New(cfg, tr, d, b, &fakeWatchdog{}, log.New(io.Discard, "", 0))
}

func TestProjectsEndpoint(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var projects []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0]["name"] != "test" {
		t.Fatalf("unexpected project name: %v", projects[0]["name"])
	}
}

func TestTicketDoneEndpoint(t *testing.T) {
	s := newTestServer(t)
	body := bytes.NewBufferString(`{"sha":"abc123"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/test/tickets/sw-07/done", body)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["signal"] == nil {
		t.Fatalf("expected signal in response: %v", payload)
	}
}

func TestWatchdogEndpoints(t *testing.T) {
	s := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/watchdog/status", nil)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/watchdog/log?lines=1", nil)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/watchdog/run", nil)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestTicketOutputSSE(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/test/tickets/sw-07/output", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		s.Router().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	body := rec.Body.String()
	if body == "" || !bytes.Contains([]byte(body), []byte("event: output")) {
		t.Fatalf("expected output event in body, got %q", body)
	}
}

func TestProjectStatusIncludesRunLevelPostBuildProgress(t *testing.T) {
	tr := tracker.New("test", map[string]tracker.Ticket{
		"cache-1":      {Status: tracker.StatusDone, Phase: 1, Type: "feature", Feature: "cache", RunID: "run-1"},
		"billing-1":    {Status: tracker.StatusRunning, Phase: 1, Type: "feature", Feature: "billing", RunID: "run-1"},
		"int-run-1":    {Status: tracker.StatusDone, Phase: 2, Type: "integration", RunID: "run-1"},
		"review-run-1": {Status: tracker.StatusDone, Phase: 3, Type: "review", RunID: "run-1"},
		"sec-run-1":    {Status: tracker.StatusRunning, Phase: 3, Type: "sec", RunID: "run-1"},
	})
	cfg := config.Default()
	cfg.Project.Name = "test"
	cfg.PostBuild.Order = []string{"review", "sec", "doc"}
	d := dispatcher.New(cfg, tr)
	s := New(cfg, tr, d, &fakeBackend{}, &fakeWatchdog{}, log.New(io.Discard, "", 0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/test/status", nil)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := payload["feature_stats"]; !ok {
		t.Fatalf("expected feature_stats in payload, got %v", payload)
	}
	rawRun, ok := payload["run_progress"]
	if !ok {
		t.Fatalf("expected run_progress in payload, got %v", payload)
	}
	runProgress, ok := rawRun.(map[string]any)
	if !ok {
		t.Fatalf("run_progress type = %T, want object", rawRun)
	}
	if got := runProgress["integration_status"]; got != "done" {
		t.Fatalf("integration_status = %v, want done", got)
	}
	rawSteps, ok := runProgress["post_build_steps"]
	if !ok {
		t.Fatalf("expected post_build_steps in run_progress, got %v", runProgress)
	}
	steps, ok := rawSteps.(map[string]any)
	if !ok {
		t.Fatalf("post_build_steps type = %T, want object", rawSteps)
	}
	if got := steps["review"]; got != "done" {
		t.Fatalf("review step status = %v, want done", got)
	}
	if got := steps["sec"]; got != "running" {
		t.Fatalf("sec step status = %v, want running", got)
	}
	if got := steps["doc"]; got != "skipped" {
		t.Fatalf("doc step status = %v, want skipped", got)
	}
}
