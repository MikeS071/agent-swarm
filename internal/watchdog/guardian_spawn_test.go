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

func TestSpawnTicketGuardianEnforceBlocksAndWritesEvidence(t *testing.T) {
	repo := initRepo(t)
	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-01.md"), `# SW-01

## Objective
implement feature

## Files to touch
- internal/watchdog/watchdog.go

## Verify
`+"```bash"+`
go test ./internal/watchdog/...
`+"```"+`

## Done Definition
done
`)

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: ""},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:                "proj",
			Repo:                repo,
			BaseBranch:          "main",
			PromptDir:           promptDir,
			Tracker:             trackerPath,
			MaxAgents:           1,
			RequireExplicitRole: true,
			RequireVerifyCmd:    true,
		},
		Guardian: config.GuardianConfig{Enabled: true, Mode: "enforce"},
		Backend:  config.BackendConfig{Type: "codex-tmux", Model: "m", Effort: "e"},
	}

	be := &fakeBackend{}
	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, dispatcher.New(cfg, tr), be, wt, &fakeNotifier{})
	w.SetGuardian(guardian.NewStrictEvaluator())

	err := w.SpawnTicket(context.Background(), "sw-01")
	if err == nil {
		t.Fatal("expected guardian block error")
	}
	if !strings.Contains(err.Error(), "guardian blocked spawn") {
		t.Fatalf("error = %q, want guardian blocked spawn", err)
	}
	if len(be.spawnCalls) != 0 {
		t.Fatalf("spawn calls = %d, want 0", len(be.spawnCalls))
	}

	events := readEventsFile(t, filepath.Join(filepath.Dir(trackerPath), "events.jsonl"))
	if len(events) == 0 {
		t.Fatal("expected guardian_block event")
	}
	var found bool
	for _, ev := range events {
		if ev.Type != "guardian_block" {
			continue
		}
		if ev.Data["event"] != "before_spawn" {
			continue
		}
		if ev.Data["rule"] != "ticket_has_required_fields" {
			t.Fatalf("rule = %v, want ticket_has_required_fields", ev.Data["rule"])
		}
		evidencePath, _ := ev.Data["evidence"].(string)
		if strings.TrimSpace(evidencePath) == "" {
			t.Fatalf("expected evidence path in event: %#v", ev.Data)
		}
		if _, statErr := os.Stat(evidencePath); statErr != nil {
			t.Fatalf("evidence file %q missing: %v", evidencePath, statErr)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected guardian_block before_spawn event, got %#v", events)
	}
}

func TestSpawnTicketGuardianAdvisoryAllowsAndEmitsWarning(t *testing.T) {
	repo := initRepo(t)
	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-01.md"), `# SW-01

## Objective
implement feature

## Files to touch
- internal/watchdog/watchdog.go

## Verify
`+"```bash"+`
go test ./internal/watchdog/...
`+"```"+`

## Done Definition
done
`)

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: ""},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:                "proj",
			Repo:                repo,
			BaseBranch:          "main",
			PromptDir:           promptDir,
			Tracker:             trackerPath,
			MaxAgents:           1,
			RequireExplicitRole: true,
			RequireVerifyCmd:    true,
		},
		Guardian: config.GuardianConfig{Enabled: true, Mode: "advisory"},
		Backend:  config.BackendConfig{Type: "codex-tmux", Model: "m", Effort: "e"},
	}

	be := &fakeBackend{}
	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, dispatcher.New(cfg, tr), be, wt, &fakeNotifier{})
	w.SetGuardian(guardian.NewStrictEvaluator())

	if err := w.SpawnTicket(context.Background(), "sw-01"); err != nil {
		t.Fatalf("SpawnTicket() advisory mode should pass, got %v", err)
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(be.spawnCalls))
	}

	events := readEventsFile(t, filepath.Join(filepath.Dir(trackerPath), "events.jsonl"))
	if len(events) == 0 {
		t.Fatal("expected guardian_warn event")
	}
	var found bool
	for _, ev := range events {
		if ev.Type != "guardian_warn" {
			continue
		}
		if ev.Data["event"] != "before_spawn" {
			continue
		}
		if ev.Data["rule"] != "ticket_has_required_fields" {
			t.Fatalf("rule = %v, want ticket_has_required_fields", ev.Data["rule"])
		}
		evidencePath, _ := ev.Data["evidence"].(string)
		if strings.TrimSpace(evidencePath) == "" {
			t.Fatalf("expected evidence path in event: %#v", ev.Data)
		}
		if _, statErr := os.Stat(evidencePath); statErr != nil {
			t.Fatalf("evidence file %q missing: %v", evidencePath, statErr)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected guardian_warn before_spawn event, got %#v", events)
	}
}

func readEventsFile(t *testing.T, path string) []Event {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	out := make([]Event, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal event line: %v", err)
		}
		out = append(out, ev)
	}
	return out
}
