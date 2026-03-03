package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type fakeSessionBackend struct {
	fakeBackend
	sessions []string
	err      error
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

func (f *fakeSessionBackend) ListSessions(context.Context) ([]string, error) {
	return f.sessions, f.err
}

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
	if err := tr.SaveTo(trackerPath); err != nil {
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
	if err := tr.SaveTo(trackerPath); err != nil {
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
	if err := tr.SaveTo(trackerPath); err != nil {
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
	if err := tr.SaveTo(trackerPath); err != nil {
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

func TestRunOnceAutoCreatesPostBuildTickets(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	trackerPath := filepath.Join(t.TempDir(), "tracker.json")

	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"cch-01": {Status: tracker.StatusDone, Phase: 2, Branch: "feat/cch-01"},
		"cch-02": {Status: tracker.StatusDone, Phase: 2, Branch: "feat/cch-02"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
		PostBuild: config.PostBuildConfig{
			Order: []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			ParallelGroups: [][]string{
				{"gap", "tst"},
				{"review", "sec"},
				{"doc", "clean"},
			},
		},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	tests := []struct {
		id       string
		status   string
		depends  []string
		profile  string
		ticketTy string
	}{
		{id: "int-cch", status: tracker.StatusRunning, depends: []string{"cch-01", "cch-02"}, ticketTy: "int"},
		{id: "gap-cch", status: tracker.StatusTodo, depends: []string{"int-cch"}, profile: "code-reviewer", ticketTy: "gap"},
		{id: "tst-cch", status: tracker.StatusTodo, depends: []string{"int-cch"}, profile: "e2e-runner", ticketTy: "tst"},
		{id: "review-cch", status: tracker.StatusTodo, depends: []string{"gap-cch", "tst-cch"}, profile: "code-reviewer", ticketTy: "review"},
		{id: "sec-cch", status: tracker.StatusTodo, depends: []string{"gap-cch", "tst-cch"}, profile: "security-reviewer", ticketTy: "sec"},
		{id: "doc-cch", status: tracker.StatusTodo, depends: []string{"review-cch", "sec-cch"}, profile: "doc-updater", ticketTy: "doc"},
		{id: "clean-cch", status: tracker.StatusTodo, depends: []string{"review-cch", "sec-cch"}, profile: "refactor-cleaner", ticketTy: "clean"},
		{id: "mem-cch", status: tracker.StatusTodo, depends: []string{"clean-cch", "doc-cch"}, profile: "code-reviewer", ticketTy: "mem"},
	}

	for _, tc := range tests {
		tk, ok := tr.Tickets[tc.id]
		if !ok {
			t.Fatalf("expected ticket %s to be auto-created", tc.id)
		}
		if tk.Status != tc.status {
			t.Fatalf("%s status = %q, want %q", tc.id, tk.Status, tc.status)
		}
		if tk.Phase != 2 {
			t.Fatalf("%s phase = %d, want 2", tc.id, tk.Phase)
		}
		if tk.Feature != "cch" {
			t.Fatalf("%s feature = %q, want cch", tc.id, tk.Feature)
		}
		if tk.Type != tc.ticketTy {
			t.Fatalf("%s type = %q, want %q", tc.id, tk.Type, tc.ticketTy)
		}
		if tk.Profile != tc.profile {
			t.Fatalf("%s profile = %q, want %q", tc.id, tk.Profile, tc.profile)
		}
		assertSameDeps(t, tc.id, tk.Depends, tc.depends)
	}

	if len(be.spawnCalls) != 1 || be.spawnCalls[0].TicketID != "int-cch" {
		t.Fatalf("expected one spawn for int-cch, got %#v", be.spawnCalls)
	}
}

func TestRunOncePostBuildAutoCreationDisabledByDefault(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	trackerPath := filepath.Join(t.TempDir(), "tracker.json")

	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"cch-01": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/cch-01"},
		"cch-02": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/cch-02"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  filepath.Join(t.TempDir(), "prompts"),
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

	if _, ok := tr.Tickets["int-cch"]; ok {
		t.Fatalf("did not expect int-cch without post_build config")
	}
	if len(be.spawnCalls) != 0 {
		t.Fatalf("expected no spawns, got %#v", be.spawnCalls)
	}
}

func TestRunOncePostBuildAutoCreationIsIdempotent(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	trackerPath := filepath.Join(t.TempDir(), "tracker.json")

	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"cch-01": {Status: tracker.StatusDone, Phase: 3, Branch: "feat/cch-01"},
		"cch-02": {Status: tracker.StatusDone, Phase: 3, Branch: "feat/cch-02"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
		PostBuild: config.PostBuildConfig{
			Order: []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			ParallelGroups: [][]string{
				{"gap", "tst"},
				{"review", "sec"},
				{"doc", "clean"},
			},
		},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("second run once: %v", err)
	}

	if len(tr.Tickets) != 10 {
		t.Fatalf("ticket count = %d, want 10", len(tr.Tickets))
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("expected one spawn across both passes, got %d", len(be.spawnCalls))
	}
}

func TestRunOncePostBuildAutoCreationDryRunNoMutation(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	trackerPath := filepath.Join(t.TempDir(), "tracker.json")

	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"cch-01": {Status: tracker.StatusDone, Phase: 2, Branch: "feat/cch-01"},
		"cch-02": {Status: tracker.StatusDone, Phase: 2, Branch: "feat/cch-02"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
		PostBuild: config.PostBuildConfig{
			Order: []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			ParallelGroups: [][]string{
				{"gap", "tst"},
				{"review", "sec"},
				{"doc", "clean"},
			},
		},
	}

	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)
	w.SetDryRun(true)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	if len(tr.Tickets) != 2 {
		t.Fatalf("dry-run mutated tracker: ticket count = %d, want 2", len(tr.Tickets))
	}
	if _, ok := tr.Tickets["int-cch"]; ok {
		t.Fatalf("dry-run should not create post-build tickets")
	}
	if len(be.spawnCalls) != 0 {
		t.Fatalf("dry-run should not spawn tickets, got %#v", be.spawnCalls)
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

func TestWatchdogHelpers(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusRunning, StartedAt: time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)},
		"sw-02": {Status: tracker.StatusTodo},
	})

	cfg := config.Default()
	cfg.Watchdog.MaxRetries = 3
	cfg.Watchdog.MaxRuntime = "1m"

	w := New(cfg, tr, nil, &fakeBackend{alive: map[string]bool{"swarm-sw-01": true}}, worktree.New(t.TempDir(), filepath.Join(t.TempDir(), "wts"), "main"), &fakeNotifier{})
	if got := w.maxRetries(); got != 3 {
		t.Fatalf("maxRetries() = %d, want 3", got)
	}
	if !w.runtimeExceeded(tr.Tickets["sw-01"].StartedAt) {
		t.Fatalf("runtimeExceeded should be true for startedAt older than max runtime")
	}
	if w.runtimeExceeded("invalid") {
		t.Fatalf("runtimeExceeded should be false for invalid timestamps")
	}
}

func TestRunningAgentCountUsesSessionLister(t *testing.T) {
	w := &Watchdog{
		backend: &fakeSessionBackend{sessions: []string{"swarm-a", "swarm-b"}},
		tracker: tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{}),
	}
	count, err := w.runningAgentCount(context.Background())
	if err != nil {
		t.Fatalf("runningAgentCount() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("runningAgentCount() = %d, want 2", count)
	}
}

func TestBuildPostBuildStages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		order          []string
		parallelGroups [][]string
		want           [][]string
	}{
		{
			name:           "contiguous groups form parallel stages",
			order:          []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			parallelGroups: [][]string{{"gap", "tst"}, {"review", "sec"}, {"doc", "clean"}},
			want:           [][]string{{"int"}, {"gap", "tst"}, {"review", "sec"}, {"doc", "clean"}, {"mem"}},
		},
		{
			name:           "non contiguous groups are ignored",
			order:          []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			parallelGroups: [][]string{{"gap", "review"}},
			want:           [][]string{{"int"}, {"gap"}, {"tst"}, {"review"}, {"sec"}, {"doc"}, {"clean"}, {"mem"}},
		},
		{
			name:           "overlapping groups are ignored after first valid group",
			order:          []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"},
			parallelGroups: [][]string{{"gap", "tst"}, {"tst", "review"}},
			want:           [][]string{{"int"}, {"gap", "tst"}, {"review"}, {"sec"}, {"doc"}, {"clean"}, {"mem"}},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildPostBuildStages(tc.order, tc.parallelGroups)
			if len(got) != len(tc.want) {
				t.Fatalf("stage count = %d, want %d (got=%v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if strings.Join(got[i], ",") != strings.Join(tc.want[i], ",") {
					t.Fatalf("stage %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
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

func assertSameDeps(t *testing.T, ticketID string, got, want []string) {
	t.Helper()
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)
	if strings.Join(gotCopy, ",") != strings.Join(wantCopy, ",") {
		t.Fatalf("%s deps = %v, want %v", ticketID, gotCopy, wantCopy)
	}
}

func TestAssemblePromptLayers(t *testing.T) {
	tmp := t.TempDir()

	// Create project structure
	os.MkdirAll(filepath.Join(tmp, "swarm", "prompts"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".agents", "profiles"), 0o755)

	// Layer 1: AGENTS.md
	os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("# Governance Rules"), 0o644)

	// Layer 2: spec file
	os.WriteFile(filepath.Join(tmp, "SPEC.md"), []byte("# Project Spec"), 0o644)

	// Layer 3: profile
	os.WriteFile(filepath.Join(tmp, ".agents", "profiles", "code-agent.md"), []byte("# Code Agent Profile"), 0o644)

	// Layer 5: footer
	os.WriteFile(filepath.Join(tmp, "swarm", "prompt-footer.md"), []byte("# Footer"), 0o644)

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:           "test",
			Tracker:        filepath.Join(tmp, "swarm", "tracker.json"),
			PromptDir:      filepath.Join(tmp, "swarm", "prompts"),
			SpecFile:       "SPEC.md",
			DefaultProfile: "code-agent",
		},
	}

	w := &Watchdog{config: cfg}

	tk := tracker.Ticket{Profile: ""}
	ticketPrompt := []byte("# mc-01\n\nImplement the thing")

	result := string(w.assemblePrompt(tk, ticketPrompt))

	// Check all layers present
	if !strings.Contains(result, "# Governance Rules") {
		t.Error("missing AGENTS.md layer")
	}
	if !strings.Contains(result, "# Project Spec") {
		t.Error("missing spec layer")
	}
	if !strings.Contains(result, "# Code Agent Profile") {
		t.Error("missing profile layer")
	}
	if !strings.Contains(result, "Implement the thing") {
		t.Error("missing ticket prompt layer")
	}
	if !strings.Contains(result, "# Footer") {
		t.Error("missing footer layer")
	}

	// Check order: governance before spec before profile before ticket
	govIdx := strings.Index(result, "Governance Rules")
	specIdx := strings.Index(result, "Project Spec")
	profIdx := strings.Index(result, "Code Agent Profile")
	ticketIdx := strings.Index(result, "Implement the thing")
	footerIdx := strings.Index(result, "# Footer")

	if govIdx >= specIdx {
		t.Error("governance should come before spec")
	}
	if specIdx >= profIdx {
		t.Error("spec should come before profile")
	}
	if profIdx >= ticketIdx {
		t.Error("profile should come before ticket")
	}
	if ticketIdx >= footerIdx {
		t.Error("ticket should come before footer")
	}
}

func TestAssemblePromptTicketProfileOverridesDefault(t *testing.T) {
	tmp := t.TempDir()

	os.MkdirAll(filepath.Join(tmp, "swarm", "prompts"), 0o755)
	os.MkdirAll(filepath.Join(tmp, ".agents", "profiles"), 0o755)

	os.WriteFile(filepath.Join(tmp, ".agents", "profiles", "code-agent.md"), []byte("DEFAULT PROFILE"), 0o644)
	os.WriteFile(filepath.Join(tmp, ".agents", "profiles", "security-reviewer.md"), []byte("SECURITY PROFILE"), 0o644)

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:           "test",
			Tracker:        filepath.Join(tmp, "swarm", "tracker.json"),
			PromptDir:      filepath.Join(tmp, "swarm", "prompts"),
			DefaultProfile: "code-agent",
		},
	}

	w := &Watchdog{config: cfg}

	// Ticket with explicit profile should override default
	tk := tracker.Ticket{Profile: "security-reviewer"}
	result := string(w.assemblePrompt(tk, []byte("task")))

	if strings.Contains(result, "DEFAULT PROFILE") {
		t.Error("should NOT contain default profile when ticket has explicit profile")
	}
	if !strings.Contains(result, "SECURITY PROFILE") {
		t.Error("should contain ticket-level profile")
	}
}

func TestAssemblePromptNoProfileGraceful(t *testing.T) {
	tmp := t.TempDir()

	os.MkdirAll(filepath.Join(tmp, "swarm", "prompts"), 0o755)

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:      "test",
			Tracker:   filepath.Join(tmp, "swarm", "tracker.json"),
			PromptDir: filepath.Join(tmp, "swarm", "prompts"),
		},
	}

	w := &Watchdog{config: cfg}

	// No profile, no AGENTS.md, no spec — should still work with just the ticket
	tk := tracker.Ticket{}
	result := string(w.assemblePrompt(tk, []byte("just the task")))

	if result != "just the task" {
		t.Errorf("expected just the ticket prompt, got: %s", result)
	}
}
