package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptsValidateStrictJSONIncludesTicketAndRule(t *testing.T) {
	repo, cfgPath := setupPromptsValidateProject(t)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-04": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/tp-04", "desc": "prompt validate"}
  }
}`)
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompts", "tp-04.md"), []byte(`# TP-04

## Objective
TODO
`), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict", "--json")
	if err == nil {
		t.Fatal("expected strict validation to fail")
	}

	var payload struct {
		Valid    bool `json:"valid"`
		Failures []struct {
			Ticket string `json:"ticket"`
			Rule   string `json:"rule"`
		} `json:"failures"`
	}
	if uerr := json.Unmarshal([]byte(jsonPortion(out)), &payload); uerr != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", uerr, out)
	}
	if payload.Valid {
		t.Fatal("expected payload.valid=false")
	}
	if len(payload.Failures) == 0 {
		t.Fatal("expected at least one failure")
	}
	if payload.Failures[0].Ticket != "tp-04" {
		t.Fatalf("failure ticket = %q, want %q", payload.Failures[0].Ticket, "tp-04")
	}
	if payload.Failures[0].Rule == "" {
		t.Fatal("expected failure rule to be populated")
	}
}

func TestPromptsValidateNonStrictDoesNotFail(t *testing.T) {
	repo, cfgPath := setupPromptsValidateProject(t)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-04": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/tp-04", "desc": "prompt validate"}
  }
}`)
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompts", "tp-04.md"), []byte(`# TP-04

## Objective
TODO
`), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--json")
	if err != nil {
		t.Fatalf("expected non-strict validation to pass, got %v", err)
	}

	var payload struct {
		Valid bool `json:"valid"`
	}
	if uerr := json.Unmarshal([]byte(jsonPortion(out)), &payload); uerr != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", uerr, out)
	}
	if payload.Valid {
		t.Fatal("expected payload.valid=false due to reported violations")
	}
}

func TestPromptsValidateTicketFlagScopesValidation(t *testing.T) {
	repo, cfgPath := setupPromptsValidateProject(t)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-good": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/tp-good", "desc": "good"},
    "tp-bad": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/tp-bad", "desc": "bad"}
  }
}`)
	goodPrompt := `# TP-GOOD

## Objective
Implement strict prompt validation.

## Files to touch
- cmd/prompts_cmd.go

## Implementation Steps
1. Add command.
2. Add tests.

## Verify
go test ./cmd/... ./internal/...

## Constraints
- Keep compatibility.
`
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompts", "tp-good.md"), []byte(goodPrompt), 0o644); err != nil {
		t.Fatalf("write good prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompts", "tp-bad.md"), []byte("# TP-BAD\n\n## Objective\nTODO\n"), 0o644); err != nil {
		t.Fatalf("write bad prompt: %v", err)
	}

	if _, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict", "--ticket", "tp-good"); err != nil {
		t.Fatalf("expected tp-good to pass strict validation, got %v", err)
	}
	if _, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict", "--ticket", "tp-bad"); err == nil {
		t.Fatal("expected tp-bad to fail strict validation")
	}
}

func setupPromptsValidateProject(t *testing.T) (string, string) {
	t.Helper()
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

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	return repo, cfgPath
}

func jsonPortion(out string) string {
	if idx := strings.Index(out, "\nError: "); idx >= 0 {
		return out[:idx]
	}
	return out
}
