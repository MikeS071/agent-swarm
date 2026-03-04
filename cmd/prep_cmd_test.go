package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepCommandPipelineHappyPath(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/sw-01",
      "desc": "ticket",
      "profile": "code-agent",
      "verify_cmd": "go test ./..."
    }
  }
}`)
	writePath(t, filepath.Join(repo, "swarm", "prompts", "sw-01.md"), "# sw-01\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err != nil {
		t.Fatalf("prep returned error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "prep ok") {
		t.Fatalf("expected prep ok output, got:\n%s", out)
	}
	if !strings.Contains(out, "tickets lint") {
		t.Fatalf("expected pipeline step output for tickets lint, got:\n%s", out)
	}
	if !strings.Contains(out, "prompts build --all") {
		t.Fatalf("expected pipeline step output for prompts build --all, got:\n%s", out)
	}
	if !strings.Contains(out, "prompts validate --strict") {
		t.Fatalf("expected pipeline step output for prompts validate --strict, got:\n%s", out)
	}
	if !strings.Contains(out, "spawnability checks") {
		t.Fatalf("expected pipeline step output for spawnability checks, got:\n%s", out)
	}
}

func TestPrepCommandPipelineFailureActionableOutput(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": ["sw-99"],
      "branch": "feat/sw-01",
      "desc": "ticket"
    }
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err == nil {
		t.Fatalf("expected prep error, got success:\n%s", out)
	}
	if !strings.Contains(err.Error(), "prep failed") {
		t.Fatalf("expected prep failed error, got: %v", err)
	}
	if !strings.Contains(out, "tickets lint") {
		t.Fatalf("expected tickets lint failure in output, got:\n%s", out)
	}
	if !strings.Contains(out, "unknown dependency") {
		t.Fatalf("expected actionable unknown dependency output, got:\n%s", out)
	}
	if !strings.Contains(out, "run `agent-swarm prep --json`") {
		t.Fatalf("expected actionable follow-up output, got:\n%s", out)
	}
}

func TestPrepCommandJSONReportsStepFailures(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/sw-01",
      "desc": "ticket"
    }
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "prep", "--json")
	if err == nil {
		t.Fatalf("expected prep error for missing required fields")
	}

	var payload struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Step   string `json:"step"`
			Ticket string `json:"ticket"`
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"issues"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("unmarshal prep json: %v\noutput:\n%s", jsonErr, out)
	}
	if payload.OK {
		t.Fatalf("expected ok=false payload")
	}
	if len(payload.Issues) == 0 {
		t.Fatalf("expected non-empty issues")
	}
	hasStep := false
	for _, issue := range payload.Issues {
		if strings.TrimSpace(issue.Step) != "" {
			hasStep = true
			break
		}
	}
	if !hasStep {
		t.Fatalf("expected at least one issue to include pipeline step, issues=%#v", payload.Issues)
	}
}

type prepProjectOptions struct {
	backendType string
}

func setupPrepProject(t *testing.T, opts prepProjectOptions) (string, string) {
	t.Helper()

	backendType := opts.backendType
	if strings.TrimSpace(backendType) == "" {
		backendType = "codex-tmux"
	}

	repo := t.TempDir()
	cfgPath := filepath.Join(repo, "swarm.toml")
	cfg := `[project]
name = "agent-swarm"
repo = "."
base_branch = "main"
max_agents = 1
min_ram_mb = 128
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
require_explicit_role = true
require_verify_cmd = true

[backend]
type = "` + backendType + `"

[notifications]
type = "stdout"

[watchdog]
interval = "5m"
`
	writePath(t, cfgPath, cfg)
	writePath(t, filepath.Join(repo, "swarm", "prompts", ".keep"), "")
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), `{"project":"agent-swarm","tickets":{}}`)
	return repo, cfgPath
}
