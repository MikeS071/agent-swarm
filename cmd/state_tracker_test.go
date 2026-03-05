package cmd

import (
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestDetectTrackerDivergence(t *testing.T) {
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".local", "state")
	activePath := filepath.Join(stateDir, "tracker.json")
	legacyPath := filepath.Join(repo, "swarm", "tracker.json")

	active := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusTodo, Phase: 1, Branch: "feat/a"},
	})
	legacy := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/a", SHA: "abc1234"},
	})
	if err := active.SaveTo(activePath); err != nil {
		t.Fatal(err)
	}
	if err := legacy.SaveTo(legacyPath); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Project.Repo = repo
	cfg.Project.StateDir = stateDir

	div, err := detectTrackerDivergence(cfg, activePath, active)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if div == nil {
		t.Fatal("expected divergence, got nil")
	}
}

func TestDetectTrackerDivergence_NoDivergence(t *testing.T) {
	repo := t.TempDir()
	stateDir := filepath.Join(repo, ".local", "state")
	activePath := filepath.Join(stateDir, "tracker.json")
	legacyPath := filepath.Join(repo, "swarm", "tracker.json")

	tr := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusTodo, Phase: 1, Branch: "feat/a"},
	})
	if err := tr.SaveTo(activePath); err != nil {
		t.Fatal(err)
	}
	if err := tr.SaveTo(legacyPath); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Project.Repo = repo
	cfg.Project.StateDir = stateDir

	div, err := detectTrackerDivergence(cfg, activePath, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if div != nil {
		t.Fatalf("expected no divergence, got: %v", div)
	}
}
