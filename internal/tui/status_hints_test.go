package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestRenderListShowsPhaseGateHints(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"g1": {Status: tracker.StatusDone, Phase: 1, Desc: "phase 1"},
		"g2": {Status: tracker.StatusTodo, Phase: 2, Desc: "phase 2"},
	})

	m := model{
		config: &config.Config{Project: config.ProjectConfig{
			Name:       "proj",
			MaxAgents:  4,
			AutoApprove: false,
			Tracker:    "/tmp/state/tracker.json",
			StateDir:   "/tmp/state",
			Repo:       "/tmp/repo",
		}},
		tracker:  tr,
		projects: []projectContext{{trackerPath: "/tmp/state/tracker.json"}},
		pageSize: 20,
		width:    120,
	}
	m.rebuildRows()

	out := m.renderList()
	if !strings.Contains(out, "Signal: PHASE_GATE") {
		t.Fatalf("expected phase-gate signal in TUI output:\n%s", out)
	}
	if !strings.Contains(out, "blocked_reason=PHASE_GATE") {
		t.Fatalf("expected blocked_reason in TUI output:\n%s", out)
	}
	if !strings.Contains(out, "next_step=run `agent-swarm go` to approve and continue") {
		t.Fatalf("expected next action hint in TUI output:\n%s", out)
	}
}

func TestRenderListShowsSpawnHintsLine(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"g1": {Status: tracker.StatusTodo, Phase: 1, Desc: "phase 1"},
	})

	m := model{
		config: &config.Config{Project: config.ProjectConfig{
			Name:        "proj",
			MaxAgents:   4,
			AutoApprove: false,
			Tracker:     "/tmp/state/tracker.json",
			StateDir:    "/tmp/state",
			Repo:        "/tmp/repo",
		}},
		tracker:  tr,
		projects: []projectContext{{trackerPath: "/tmp/state/tracker.json"}},
		pageSize: 20,
		width:    120,
	}
	m.rebuildRows()

	out := m.renderList()
	if !strings.Contains(out, "Signal: SPAWN") {
		t.Fatalf("expected spawn signal in TUI output:\n%s", out)
	}
	if !strings.Contains(out, "blocked_reason=NONE") {
		t.Fatalf("expected blocked_reason=NONE in TUI output:\n%s", out)
	}
	if !strings.Contains(out, "next_step=spawnable tickets available") {
		t.Fatalf("expected next_step for spawn in TUI output:\n%s", out)
	}
}

func TestToggleAutoApproveUpdatesConfigAndTitle(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "swarm.toml")
	trackerPath := filepath.Join(tmp, "swarm", "tracker.json")
	if err := os.MkdirAll(filepath.Dir(trackerPath), 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	if err := os.WriteFile(trackerPath, []byte(`{"project":"proj","tickets":{}}`), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	cfgRaw := `[project]
name = "proj"
repo = "."
base_branch = "main"
max_agents = 2
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"
`
	if err := os.WriteFile(cfgPath, []byte(cfgRaw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, tr, err := loadProject(projectContext{configPath: cfgPath})
	if err != nil {
		t.Fatalf("loadProject: %v", err)
	}
	m := model{
		config:       cfg,
		tracker:      tr,
		projects:     []projectContext{{configPath: cfgPath, trackerPath: cfg.Project.Tracker, name: cfg.Project.Name}},
		projectIndex: 0,
		pageSize:     20,
		width:        120,
	}
	m.rebuildRows()

	m.toggleAutoApprove()
	if !m.config.Project.AutoApprove {
		t.Fatalf("expected auto_approve true after toggle")
	}
	if out := m.renderList(); !strings.Contains(out, "[auto]") {
		t.Fatalf("expected title bar to show [auto], got:\n%s", out)
	}
	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if !strings.Contains(string(updated), "auto_approve = true") {
		t.Fatalf("expected config file updated with auto_approve=true:\n%s", string(updated))
	}
}

func TestLoadProjectFailsOnTrackerDivergence(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "swarm.toml")
	legacyPath := filepath.Join(tmp, "swarm", "tracker.json")
	statePath := filepath.Join(tmp, ".local", "state", "tracker.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"project":"proj","tickets":{"a":{"status":"done","phase":1}}}`), 0o644); err != nil {
		t.Fatalf("write legacy tracker: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{"project":"proj","tickets":{"a":{"status":"done","phase":1},"b":{"status":"todo","phase":2}}}`), 0o644); err != nil {
		t.Fatalf("write state tracker: %v", err)
	}
	cfgRaw := `[project]
name = "proj"
repo = "."
state_dir = ".local/state"
base_branch = "main"
max_agents = 2
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"
`
	if err := os.WriteFile(cfgPath, []byte(cfgRaw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, _, err := loadProject(projectContext{configPath: cfgPath})
	if err == nil {
		t.Fatal("expected divergence error, got nil")
	}
	if !strings.Contains(err.Error(), "tracker divergence detected") {
		t.Fatalf("expected divergence error, got: %v", err)
	}
}
