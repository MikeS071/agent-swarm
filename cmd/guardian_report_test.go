package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupGuardianReportProject(t *testing.T) (repo, cfgPath, evidenceDir string) {
	t.Helper()
	repo = t.TempDir()
	cfgPath = filepath.Join(repo, "swarm.toml")
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	if err := os.MkdirAll(filepath.Dir(trackerPath), 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	if err := os.WriteFile(trackerPath, []byte(`{"project":"proj","tickets":{}}`), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	cfg := `[project]
name = "proj"
repo = "."
base_branch = "main"
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	evidenceDir = filepath.Join(repo, "swarm", "guardian", "evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	return repo, cfgPath, evidenceDir
}

func TestGuardianReportJSON(t *testing.T) {
	_, cfgPath, evidenceDir := setupGuardianReportProject(t)
	a := `{"timestamp":"2026-03-05T11:00:00Z","event":"before_spawn","ticket_id":"g4-02","result":"BLOCK","rule_id":"ticket_desc_has_scope_and_verify","reason":"missing scope/verify"}`
	b := `{"timestamp":"2026-03-05T11:01:00Z","event":"before_mark_done","ticket_id":"g4-02","result":"WARN","rule_id":"prompt_template_sections","reason":"missing section"}`
	if err := os.WriteFile(filepath.Join(evidenceDir, "a.json"), []byte(a), 0o644); err != nil {
		t.Fatalf("write evidence a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "b.json"), []byte(b), 0o644); err != nil {
		t.Fatalf("write evidence b: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "guardian", "report", "--json")
	if err != nil {
		t.Fatalf("guardian report json: %v", err)
	}
	var payload struct {
		Total  int `json:"total"`
		Counts struct {
			Allow int `json:"allow"`
			Warn  int `json:"warn"`
			Block int `json:"block"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse report json: %v\n%s", err, out)
	}
	if payload.Total != 2 || payload.Counts.Block != 1 || payload.Counts.Warn != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestGuardianReportText(t *testing.T) {
	_, cfgPath, evidenceDir := setupGuardianReportProject(t)
	a := `{"timestamp":"2026-03-05T11:00:00Z","event":"before_spawn","ticket_id":"g4-02","result":"BLOCK","rule_id":"ticket_desc_has_scope_and_verify","reason":"missing scope/verify"}`
	if err := os.WriteFile(filepath.Join(evidenceDir, "a.json"), []byte(a), 0o644); err != nil {
		t.Fatalf("write evidence a: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "guardian", "report")
	if err != nil {
		t.Fatalf("guardian report text: %v", err)
	}
	if !strings.Contains(out, "guardian evidence: total=1") {
		t.Fatalf("unexpected report output: %s", out)
	}
	if !strings.Contains(out, "ticket=g4-02") {
		t.Fatalf("expected ticket in output: %s", out)
	}
}
