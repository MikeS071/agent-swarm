package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptsCheckReportsMissingTodoPrompts(t *testing.T) {
	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}

	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/sw-01", "desc": "one"},
    "sw-02": {"status": "done", "phase": 1, "depends": [], "branch": "feat/sw-02", "desc": "two"},
    "sw-03": {"status": "todo", "phase": 1, "depends": ["sw-01"], "branch": "feat/sw-03", "desc": "three"}
  }
}`)
	if err := os.WriteFile(filepath.Join(promptDir, "sw-01.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	missing, err := CheckPrompts(trackerPath, promptDir)
	if err != nil {
		t.Fatalf("check prompts: %v", err)
	}
	if len(missing) != 1 || missing[0] != "sw-03" {
		t.Fatalf("unexpected missing prompts: %#v", missing)
	}
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func TestPromptsValidateStrictReportsFailures(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")

	writeJSON(t, trackerPath, `{
  "project":"agent-swarm",
  "tickets":{
    "sw-01":{"status":"todo","phase":1,"depends":[],"branch":"feat/sw-01","desc":"one"}
  }
}`)
	writePath(t, filepath.Join(promptDir, "sw-01.md"), `# sw-01
## Objective
TODO
## Dependencies
none
## Scope
- update docs
`)

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict")
	if err == nil {
		t.Fatalf("expected strict validation error, output: %s", out)
	}
	if !strings.Contains(out, "sw-01 [prompt_has_required_sections]") {
		t.Fatalf("expected required sections failure, got: %s", out)
	}
	if !strings.Contains(out, "sw-01 [prompt_no_unresolved_placeholders]") {
		t.Fatalf("expected placeholder failure, got: %s", out)
	}
	if !strings.Contains(out, "sw-01 [prompt_has_verify_command]") {
		t.Fatalf("expected verify command failure, got: %s", out)
	}
	if !strings.Contains(out, "sw-01 [prompt_has_explicit_file_scope]") {
		t.Fatalf("expected explicit file scope failure, got: %s", out)
	}
}

func TestPromptsValidateStrictPasses(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")

	writeJSON(t, trackerPath, `{
  "project":"agent-swarm",
  "tickets":{
    "sw-01":{"status":"todo","phase":1,"depends":[],"branch":"feat/sw-01","desc":"one"}
  }
}`)
	writePath(t, filepath.Join(promptDir, "sw-01.md"), `# sw-01
## Objective
Implement strict prompt validation.
## Dependencies
none
## Scope
- cmd/prompts_cmd.go
- internal/prompts/validate.go
## Verify
go test ./cmd/... -run PromptsValidate
`)

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict")
	if err != nil {
		t.Fatalf("expected strict validation success, err: %v output: %s", err, out)
	}
	if !strings.Contains(out, "prompts validation ok") {
		t.Fatalf("unexpected success output: %s", out)
	}
}

func TestPromptsValidateJSONIncludesTicketAndRule(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")

	writeJSON(t, trackerPath, `{
  "project":"agent-swarm",
  "tickets":{
    "sw-01":{"status":"todo","phase":1,"depends":[],"branch":"feat/sw-01","desc":"one"}
  }
}`)
	writePath(t, filepath.Join(promptDir, "sw-01.md"), `# sw-01
## Objective
Add details here
`)

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict", "--json")
	if err == nil {
		t.Fatalf("expected strict json validation error, output: %s", out)
	}
	var payload struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Ticket string `json:"ticket"`
			Rule   string `json:"rule"`
		} `json:"issues"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("unmarshal json output: %v\noutput: %s", jsonErr, out)
	}
	if payload.OK {
		t.Fatalf("expected ok=false payload: %+v", payload)
	}
	if len(payload.Issues) == 0 {
		t.Fatalf("expected issues in payload: %+v", payload)
	}
	if payload.Issues[0].Ticket == "" || payload.Issues[0].Rule == "" {
		t.Fatalf("issue missing ticket/rule: %+v", payload.Issues[0])
	}
}

func TestPromptsValidateTicketFilter(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")

	writeJSON(t, trackerPath, `{
  "project":"agent-swarm",
  "tickets":{
    "sw-01":{"status":"todo","phase":1,"depends":[],"branch":"feat/sw-01","desc":"one"},
    "sw-02":{"status":"todo","phase":1,"depends":[],"branch":"feat/sw-02","desc":"two"}
  }
}`)
	writePath(t, filepath.Join(promptDir, "sw-01.md"), `# sw-01
## Objective
TODO
`)
	writePath(t, filepath.Join(promptDir, "sw-02.md"), `# sw-02
## Objective
Implement strict prompt validation.
## Dependencies
none
## Scope
- cmd/prompts_cmd.go
## Verify
go test ./cmd/... -run PromptsValidate
`)

	out, err := runRootWithConfig(t, cfgPath, "prompts", "validate", "--strict", "--ticket", "sw-02")
	if err != nil {
		t.Fatalf("expected filtered validation success, err: %v output: %s", err, out)
	}
	if strings.Contains(out, "sw-01") {
		t.Fatalf("expected ticket filter to ignore sw-01, output: %s", out)
	}
}
