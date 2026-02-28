package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

type fakeBackend struct {
	spawnCalls []backend.SpawnConfig
	exited     map[string]bool
	alive      map[string]bool
}

func (f *fakeBackend) Spawn(_ context.Context, cfg backend.SpawnConfig) (backend.AgentHandle, error) {
	f.spawnCalls = append(f.spawnCalls, cfg)
	session := "swarm-" + cfg.TicketID
	if f.alive == nil {
		f.alive = map[string]bool{}
	}
	f.alive[session] = true
	return backend.AgentHandle{SessionName: session, StartedAt: time.Now()}, nil
}

func (f *fakeBackend) IsAlive(h backend.AgentHandle) bool {
	if f.alive == nil {
		return false
	}
	return f.alive[h.SessionName]
}

func (f *fakeBackend) HasExited(h backend.AgentHandle) bool {
	if f.exited == nil {
		return false
	}
	return f.exited[h.SessionName]
}

func (f *fakeBackend) GetOutput(backend.AgentHandle, int) (string, error) { return "", nil }
func (f *fakeBackend) Kill(backend.AgentHandle) error                     { return nil }
func (f *fakeBackend) Name() string                                       { return "fake" }

type fakeNotifier struct {
	alerts []string
	infos  []string
}

func (f *fakeNotifier) Alert(_ context.Context, msg string) error {
	f.alerts = append(f.alerts, msg)
	return nil
}

func (f *fakeNotifier) Info(_ context.Context, msg string) error {
	f.infos = append(f.infos, msg)
	return nil
}

func TestRunOnceMarksDoneAndChainsSpawn(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	wtPath, err := wtMgr.Create("sw-01", "feat/sw-01")
	if err != nil {
		t.Fatalf("create sw-01 worktree: %v", err)
	}
	writeFile(t, filepath.Join(wtPath, "done.txt"), "ok\n")
	runGit(t, wtPath, "add", "done.txt")
	runGit(t, wtPath, "commit", "-m", "done")

	promptDir := filepath.Join(t.TempDir(), "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-02.md"), "# sw-02\n")

	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusRunning, Phase: 1, Branch: "feat/sw-01"},
		"sw-02": {Status: tracker.StatusTodo, Phase: 1, Branch: "feat/sw-02", Depends: []string{"sw-01"}},
	})
	if err := tr.Save(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  2,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{exited: map[string]bool{"swarm-sw-01": true}}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if got := tr.Tickets["sw-01"].Status; got != tracker.StatusDone {
		t.Fatalf("sw-01 status = %q, want done", got)
	}
	if got := tr.Tickets["sw-02"].Status; got != tracker.StatusRunning {
		t.Fatalf("sw-02 status = %q, want running", got)
	}
	if len(be.spawnCalls) != 1 || be.spawnCalls[0].TicketID != "sw-02" {
		t.Fatalf("expected spawn sw-02, got %#v", be.spawnCalls)
	}
}

func TestRunOnceRespawnThenFailOnSecondNoCommitExit(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	if _, err := wtMgr.Create("sw-01", "feat/sw-01"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	promptDir := filepath.Join(t.TempDir(), "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-01.md"), "# sw-01\n")

	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusRunning, Phase: 1, Branch: "feat/sw-01"},
	})
	if err := tr.Save(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Watchdog: config.WatchdogConfig{MaxRetries: 2},
		Backend:  config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{exited: map[string]bool{"swarm-sw-01": true}}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("expected first respawn spawn call, got %d", len(be.spawnCalls))
	}
	if got := tr.Tickets["sw-01"].Status; got != tracker.StatusRunning {
		t.Fatalf("status after first run = %q, want running", got)
	}

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if got := tr.Tickets["sw-01"].Status; got != tracker.StatusFailed {
		t.Fatalf("status after second run = %q, want failed", got)
	}
	if len(n.alerts) == 0 {
		t.Fatalf("expected alert for failed ticket")
	}
}

func TestRunOnceIdleSpawnWhenNothingRunning(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-01.md"), "# sw-01\n")

	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusTodo, Phase: 1, Branch: "feat/sw-01"},
	})
	if err := tr.Save(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if got := tr.Tickets["sw-01"].Status; got != tracker.StatusRunning {
		t.Fatalf("sw-01 status = %q, want running", got)
	}
	if len(be.spawnCalls) != 1 || be.spawnCalls[0].TicketID != "sw-01" {
		t.Fatalf("expected idle spawn sw-01, got %#v", be.spawnCalls)
	}
}

func TestRunOncePhaseGateBlocksSpawning(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-02.md"), "# sw-02\n")

	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/sw-01"},
		"sw-02": {Status: tracker.StatusTodo, Phase: 2, Branch: "feat/sw-02"},
	})
	if err := tr.Save(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if len(be.spawnCalls) != 0 {
		t.Fatalf("expected no spawns at phase gate, got %#v", be.spawnCalls)
	}
	if len(n.infos) == 0 {
		t.Fatalf("expected phase gate notification")
	}
}

func TestEventLogAppendJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := NewEventLog(path)

	if err := log.Append(Event{Type: "ticket_spawned", Ticket: "sw-01", Timestamp: time.Unix(1700000000, 0), Data: map[string]any{"attempt": 1}}); err != nil {
		t.Fatalf("append: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		t.Fatal("expected one jsonl line")
	}
	var ev Event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("unmarshal line: %v", err)
	}
	if ev.Type != "ticket_spawned" || ev.Ticket != "sw-01" {
		t.Fatalf("unexpected event: %#v", ev)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "README.md"), "root\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
