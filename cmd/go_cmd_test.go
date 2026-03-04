package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestGoCommandGuardianEnforcement(t *testing.T) {
	tests := []struct {
		name               string
		flow               string
		wantErrSub         string
		wantUnlockedPhase  int
		wantGuardianResult string
	}{
		{
			name: "happy path enforce mode with valid flow allows transition",
			flow: `
version: 1
name: default-v2-flow
modes:
  default: enforce
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions:
  - from: draft
    to: planned
    requires:
      rules:
        - type: ticket_desc_has_scope_and_verify
`,
			wantUnlockedPhase:  2,
			wantGuardianResult: "ALLOW",
		},
		{
			name: "error path enforce mode with invalid flow blocks transition",
			flow: `
version: 1
name: invalid-flow
modes:
  default: enforce
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions:
  - from: draft
    to: planned
    requires:
      rules:
        - type: does_not_exist
`,
			wantErrSub:         "guardian blocked go transition",
			wantUnlockedPhase:  0,
			wantGuardianResult: "BLOCK",
		},
		{
			name: "edge path advisory mode with invalid flow warns but allows transition",
			flow: `
version: 1
name: invalid-flow
modes:
  default: advisory
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions:
  - from: draft
    to: planned
    requires:
      rules:
        - type: does_not_exist
`,
			wantUnlockedPhase:  2,
			wantGuardianResult: "WARN",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			repo, cfgPath, trackerPath := setupGoCommandTestProject(t)
			flowPath := filepath.Join(repo, "swarm", "flow.v2.yaml")
			if err := os.WriteFile(flowPath, []byte(strings.TrimSpace(tt.flow)+"\n"), 0o644); err != nil {
				t.Fatalf("write flow.v2.yaml: %v", err)
			}

			_, err := runRootWithConfig(t, cfgPath, "go")
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErrSub)
				}
			} else if err != nil {
				t.Fatalf("go command returned error: %v", err)
			}

			loaded, loadErr := tracker.Load(trackerPath)
			if loadErr != nil {
				t.Fatalf("load tracker: %v", loadErr)
			}
			if loaded.UnlockedPhase != tt.wantUnlockedPhase {
				t.Fatalf("UnlockedPhase = %d, want %d", loaded.UnlockedPhase, tt.wantUnlockedPhase)
			}

			guardianEventsPath := filepath.Join(filepath.Dir(trackerPath), "guardian-events.jsonl")
			body, readErr := os.ReadFile(guardianEventsPath)
			if readErr != nil {
				t.Fatalf("read guardian events: %v", readErr)
			}
			line := strings.TrimSpace(string(body))
			if line == "" {
				t.Fatalf("expected guardian event line in %s", guardianEventsPath)
			}
			if !strings.Contains(line, `"rule":"flow_schema_valid"`) {
				t.Fatalf("expected flow_schema_valid rule event, got %q", line)
			}
			if !strings.Contains(line, `"result":"`+tt.wantGuardianResult+`"`) {
				t.Fatalf("expected guardian result %q, got %q", tt.wantGuardianResult, line)
			}
		})
	}
}

func TestGoCommandWithoutFlowFileSkipsGuardian(t *testing.T) {
	repo, cfgPath, trackerPath := setupGoCommandTestProject(t)

	_, err := runRootWithConfig(t, cfgPath, "go")
	if err != nil {
		t.Fatalf("go command returned error: %v", err)
	}

	loaded, loadErr := tracker.Load(trackerPath)
	if loadErr != nil {
		t.Fatalf("load tracker: %v", loadErr)
	}
	if loaded.UnlockedPhase != 2 {
		t.Fatalf("UnlockedPhase = %d, want 2", loaded.UnlockedPhase)
	}

	guardianEventsPath := filepath.Join(repo, "swarm", "guardian-events.jsonl")
	_, statErr := os.Stat(guardianEventsPath)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no guardian events file when flow file is missing, stat error=%v", statErr)
	}
}

func setupGoCommandTestProject(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()

	cfgPath := filepath.Join(repo, "swarm.toml")
	cfg := `
[project]
name = "agent-swarm"
repo = "."
base_branch = "main"
max_agents = 3
min_ram_mb = 256
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

auto_approve = false

[backend]
type = "codex-tmux"
model = "gpt-5.3-codex"
binary = ""
effort = "high"
bypass_sandbox = true

[notifications]
type = "stdout"
telegram_chat_id = ""
telegram_token = ""

[watchdog]
interval = "5m"
max_runtime = "45m"
stale_timeout = "10m"
max_retries = 2
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts dir: %v", err)
	}

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("agent-swarm", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusDone, Phase: 1, Branch: "feat/sw-01"},
		"sw-02": {Status: tracker.StatusTodo, Phase: 2, Branch: "feat/sw-02", Depends: []string{"sw-01"}},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	return repo, cfgPath, trackerPath
}
