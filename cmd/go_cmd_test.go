package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestGuardianGoAllowsPhaseTransitionWhenChecksPass(t *testing.T) {
	repo := t.TempDir()
	cfgPath := filepath.Join(repo, "swarm.toml")
	writePath(t, cfgPath, guardianGoConfig(true))
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "test",
  "tickets": {
    "sw-01": {"status": "done", "phase": 1},
    "sw-02": {"status": "todo", "phase": 2, "profile": "code-agent", "verify_cmd": "go test ./..."}
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "go")
	if err != nil {
		t.Fatalf("go command error = %v", err)
	}
	if !strings.Contains(out, "Phase gate approved. Signal:") {
		t.Fatalf("expected approval output, got %q", out)
	}

	tr, err := tracker.Load(filepath.Join(repo, "swarm", "tracker.json"))
	if err != nil {
		t.Fatalf("load tracker: %v", err)
	}
	if tr.UnlockedPhase != 2 {
		t.Fatalf("unlocked_phase = %d, want 2", tr.UnlockedPhase)
	}
}

func TestGuardianGoBlocksPhaseTransitionWithExplicitUnmetConditions(t *testing.T) {
	repo := t.TempDir()
	cfgPath := filepath.Join(repo, "swarm.toml")
	writePath(t, cfgPath, guardianGoConfig(true))
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "test",
  "tickets": {
    "sw-01": {"status": "done", "phase": 1},
    "sw-02": {"status": "todo", "phase": 2}
  }
}`)

	_, err := runRootWithConfig(t, cfgPath, "go")
	if err == nil {
		t.Fatalf("expected go command to block transition")
	}
	if !strings.Contains(err.Error(), "guardian blocked phase transition") {
		t.Fatalf("expected guardian block error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ticket sw-02 missing explicit role/profile") {
		t.Fatalf("expected explicit role unmet condition, got %v", err)
	}
	if !strings.Contains(err.Error(), "ticket sw-02 missing verify_cmd") {
		t.Fatalf("expected verify_cmd unmet condition, got %v", err)
	}

	tr, loadErr := tracker.Load(filepath.Join(repo, "swarm", "tracker.json"))
	if loadErr != nil {
		t.Fatalf("load tracker: %v", loadErr)
	}
	if tr.UnlockedPhase == 2 {
		t.Fatalf("unlocked_phase = %d, expected transition to remain blocked", tr.UnlockedPhase)
	}
}

func TestGuardianGoDisabledDoesNotBlockPhaseTransition(t *testing.T) {
	repo := t.TempDir()
	cfgPath := filepath.Join(repo, "swarm.toml")
	writePath(t, cfgPath, guardianGoConfig(false))
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "test",
  "tickets": {
    "sw-01": {"status": "done", "phase": 1},
    "sw-02": {"status": "todo", "phase": 2}
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "go")
	if err != nil {
		t.Fatalf("go command error = %v", err)
	}
	if !strings.Contains(out, "Signal: BLOCKED") {
		t.Fatalf("expected blocked dispatch signal after approval, got %q", out)
	}

	tr, loadErr := tracker.Load(filepath.Join(repo, "swarm", "tracker.json"))
	if loadErr != nil {
		t.Fatalf("load tracker: %v", loadErr)
	}
	if tr.UnlockedPhase != 2 {
		t.Fatalf("unlocked_phase = %d, want 2", tr.UnlockedPhase)
	}
}

func guardianGoConfig(guardianEnabled bool) string {
	enabled := "false"
	if guardianEnabled {
		enabled = "true"
	}
	return `[project]
name = "test"
repo = "."
base_branch = "main"
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[guardian]
enabled = ` + enabled + `
`
}
