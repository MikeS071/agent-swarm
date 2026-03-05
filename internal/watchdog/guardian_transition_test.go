package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func TestGuardianTransitionAllowsAutoApproveWhenConditionsMet(t *testing.T) {
	repo := initRepo(t)
	wtMgr := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	promptDir := filepath.Join(t.TempDir(), "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-02.md"), "# sw-02\n")

	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/sw-01"},
		"sw-02": {Status: tracker.StatusTodo, Phase: 2, Branch: "feat/sw-02", Profile: "code-agent", VerifyCmd: "go test ./..."},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Repo:                repo,
			BaseBranch:          "main",
			PromptDir:           promptDir,
			Tracker:             trackerPath,
			MaxAgents:           1,
			AutoApprove:         true,
			RequireExplicitRole: true,
			RequireVerifyCmd:    true,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)
	w.SetGuardian(guardian.NewStrictEvaluator())

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(be.spawnCalls) != 1 || be.spawnCalls[0].TicketID != "sw-02" {
		t.Fatalf("expected auto-approved transition spawn, got %#v", be.spawnCalls)
	}
	if tr.UnlockedPhase != 2 {
		t.Fatalf("unlocked_phase = %d, want 2", tr.UnlockedPhase)
	}
}

func TestGuardianTransitionBlocksWithExplicitUnmetConditions(t *testing.T) {
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
			Repo:                repo,
			BaseBranch:          "main",
			PromptDir:           promptDir,
			Tracker:             trackerPath,
			MaxAgents:           1,
			AutoApprove:         true,
			RequireExplicitRole: true,
			RequireVerifyCmd:    true,
		},
		Backend: config.BackendConfig{Model: "m", Effort: "e"},
	}
	be := &fakeBackend{}
	n := &fakeNotifier{}
	d := dispatcher.New(cfg, tr)
	w := New(cfg, tr, d, be, wtMgr, n)
	w.SetGuardian(guardian.NewStrictEvaluator())

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	if len(be.spawnCalls) != 0 {
		t.Fatalf("expected blocked transition to prevent spawn, got %#v", be.spawnCalls)
	}
	if tr.UnlockedPhase == 2 {
		t.Fatalf("unlocked_phase = %d, expected blocked transition", tr.UnlockedPhase)
	}
	if len(n.alerts) == 0 {
		t.Fatalf("expected guardian block alert")
	}
	alert := strings.Join(n.alerts, "\n")
	if !strings.Contains(alert, "guardian blocked phase transition") {
		t.Fatalf("expected phase transition block alert, got %q", alert)
	}
	if !strings.Contains(alert, "ticket sw-02 missing explicit role/profile") {
		t.Fatalf("expected explicit unmet condition in alert, got %q", alert)
	}
	if !strings.Contains(alert, "ticket sw-02 missing verify_cmd") {
		t.Fatalf("expected verify unmet condition in alert, got %q", alert)
	}

	eventsPath := filepath.Join(filepath.Dir(trackerPath), "events.jsonl")
	b, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("parse event line %q: %v", line, err)
		}
		if ev.Type != "guardian_block" {
			continue
		}
		if ev.Data["event"] != "phase_transition" {
			continue
		}
		unmetRaw, ok := ev.Data["unmet_conditions"].([]any)
		if !ok {
			t.Fatalf("guardian block event missing unmet_conditions: %#v", ev.Data)
		}
		parts := make([]string, 0, len(unmetRaw))
		for _, v := range unmetRaw {
			s, _ := v.(string)
			parts = append(parts, s)
		}
		joined := strings.Join(parts, " | ")
		if !strings.Contains(joined, "ticket sw-02 missing explicit role/profile") || !strings.Contains(joined, "ticket sw-02 missing verify_cmd") {
			t.Fatalf("unexpected unmet_conditions: %#v", parts)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected guardian_block phase_transition event in %s", eventsPath)
	}
}
