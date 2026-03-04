package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func writePlanTestFiles(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	cfgPath := filepath.Join(repo, "swarm.toml")
	cfg := `[project]
name = "proj"
repo = "."
base_branch = "main"
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
features_dir = "swarm/features"
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "features"), 0o755); err != nil {
		t.Fatalf("mkdir features: %v", err)
	}
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	if err := os.WriteFile(trackerPath, []byte(`{
  "project":"proj",
  "tickets":{
    "a":{"status":"todo","phase":1},
    "b":{"status":"todo","phase":1,"depends":["a"]},
    "c":{"status":"todo","phase":1,"depends":["a"]}
  }
}`), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	return repo, cfgPath, trackerPath
}

func TestPlanOptimizeDryRunDoesNotPersist(t *testing.T) {
	t.Parallel()
	_, cfgPath, trackerPath := writePlanTestFiles(t)

	if _, err := runRootWithConfig(t, cfgPath, "plan", "optimize"); err != nil {
		t.Fatalf("plan optimize dry-run: %v", err)
	}
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		t.Fatalf("load tracker: %v", err)
	}
	if tr.Tickets["a"].Priority != 0 || tr.Tickets["b"].Priority != 0 || tr.Tickets["c"].Priority != 0 {
		t.Fatalf("expected dry-run not to persist priorities: %#v", tr.Tickets)
	}
}

func TestPlanOptimizeApplyPersists(t *testing.T) {
	t.Parallel()
	_, cfgPath, trackerPath := writePlanTestFiles(t)

	if _, err := runRootWithConfig(t, cfgPath, "plan", "optimize", "--apply"); err != nil {
		t.Fatalf("plan optimize apply: %v", err)
	}
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		t.Fatalf("load tracker: %v", err)
	}
	if tr.Tickets["a"].Priority <= tr.Tickets["b"].Priority {
		t.Fatalf("expected root ticket to get higher priority: a=%d b=%d", tr.Tickets["a"].Priority, tr.Tickets["b"].Priority)
	}
	if tr.Tickets["a"].Priority == 0 {
		t.Fatalf("expected priorities to be persisted")
	}
}
