package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGuardianValidateProject(t *testing.T) (repo, cfgPath string) {
	t.Helper()
	repo = t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "swarm"), 0o755); err != nil {
		t.Fatalf("mkdir swarm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "swarm", "tracker.json"), []byte(`{"project":"proj","tickets":{}}`), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	flow := `version: 2
mode: advisory
settings:
  fail_closed: false
  cache_ttl_seconds: 0
  max_evidence_bytes: 1024
enforcement_points: [before_spawn]
contexts:
  default:
    severity: block
rules:
  - id: ticket_desc_has_scope_and_verify
    enabled: true
    description: test
    severity: block
    enforcement_points: [before_spawn]
    target:
      kind: ticket
      source: swarm/tracker.json
      fields: [scope, verify]
    check:
      type: ticket_fields
      params: {}
    pass_when:
      op: all
      conditions:
        - metric: required_fields_present
          equals: true
    fail_reason: ticket invalid
    evidence:
      kind: json
      path: evidence/ticket.json
overrides:
  enabled: false
  require_reason: false
  require_expiry: false
  max_duration_hours: 0
  store: approvals.json
events:
  file: guardian-events.jsonl
  include: [timestamp]
`
	if err := os.WriteFile(filepath.Join(repo, "swarm", "flow.v2.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	cfgPath = filepath.Join(repo, "swarm.toml")
	cfg := `[project]
name = "proj"
repo = "."
base_branch = "main"
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]

[guardian]
enabled = true
flow_file = "swarm/flow.v2.yaml"
mode = "advisory"
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return repo, cfgPath
}

func TestGuardianValidateCommand(t *testing.T) {
	_, cfgPath := setupGuardianValidateProject(t)
	out, err := runRootWithConfig(t, cfgPath, "guardian", "validate")
	if err != nil {
		t.Fatalf("guardian validate: %v", err)
	}
	if !strings.Contains(out, "guardian policy valid") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGuardianCheckCommand(t *testing.T) {
	_, cfgPath := setupGuardianValidateProject(t)
	out, err := runRootWithConfig(t, cfgPath, "guardian", "check", "--event", "before_spawn", "--json")
	if err != nil {
		t.Fatalf("guardian check: %v", err)
	}
	if !strings.Contains(out, "\"Result\": \"WARN\"") {
		t.Fatalf("expected warn decision, got: %s", out)
	}
}
