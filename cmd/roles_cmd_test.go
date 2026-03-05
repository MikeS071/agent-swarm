package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/roles"
)

func TestRolesCheckJSONSuccess(t *testing.T) {
	repo, cfgPath := setupRolesProject(t)
	writeRoleAssetsForCmdTest(t, repo, "code-agent")

	out, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err != nil {
		t.Fatalf("roles check --json: %v\noutput: %s", err, out)
	}

	var payload struct {
		OK       bool            `json:"ok"`
		Roles    []string        `json:"roles"`
		Failures []roles.Failure `json:"failures"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, out)
	}
	if !payload.OK {
		t.Fatalf("ok = false, failures: %#v", payload.Failures)
	}
	if !reflect.DeepEqual(payload.Roles, []string{"code-agent"}) {
		t.Fatalf("roles = %#v, want %#v", payload.Roles, []string{"code-agent"})
	}
	if len(payload.Failures) != 0 {
		t.Fatalf("failures = %#v, want empty", payload.Failures)
	}
}

func TestRolesCheckJSONFailureIncludesMachineReadableFailures(t *testing.T) {
	_, cfgPath := setupRolesProject(t)

	out, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err == nil {
		t.Fatalf("expected error, output: %s", out)
	}

	var payload struct {
		OK       bool            `json:"ok"`
		Failures []roles.Failure `json:"failures"`
	}
	if jerr := json.Unmarshal([]byte(out), &payload); jerr != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", jerr, out)
	}
	if payload.OK {
		t.Fatalf("ok = true, expected false, payload: %#v", payload)
	}
	if len(payload.Failures) == 0 {
		t.Fatalf("expected at least one failure, got %#v", payload.Failures)
	}
	found := false
	for _, failure := range payload.Failures {
		if failure.Asset == roles.AssetRoleProfile && failure.Role == "code-agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing profile failure for code-agent, got %#v", payload.Failures)
	}
}

func TestRolesCheckTextFailure(t *testing.T) {
	repo, cfgPath := setupRolesProject(t)
	writeRoleAssetsForCmdTest(t, repo, "code-agent")
	missingBaseRule := filepath.Join(repo, ".codex", "rules", "common", roles.RequiredBaseRuleFiles()[0])
	if err := os.Remove(missingBaseRule); err != nil {
		t.Fatalf("remove base rule %s: %v", missingBaseRule, err)
	}

	out, err := runRootWithConfig(t, cfgPath, "roles", "check")
	if err == nil {
		t.Fatalf("expected error, output: %s", out)
	}
	if !strings.Contains(out, "base_rule") {
		t.Fatalf("expected human-readable base rule failures, got: %s", out)
	}
}

func setupRolesProject(t *testing.T) (string, string) {
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
		t.Fatalf("mkdir prompts: %v", err)
	}
	for _, name := range roles.RequiredBaseRuleFiles() {
		path := filepath.Join(repo, ".codex", "rules", "common", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir base rules dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write base rule %s: %v", path, err)
		}
	}

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project":"agent-swarm",
  "tickets":{
    "sw-01":{"status":"todo","phase":1,"depends":[],"profile":"code-agent"}
  }
}`)

	return repo, cfgPath
}

func writeRoleAssetsForCmdTest(t *testing.T, root, role string) {
	t.Helper()
	profilePath := filepath.Join(root, ".agents", "profiles", role+".md")
	rolePath := filepath.Join(root, ".agents", "roles", role+".yaml")
	rulePath := filepath.Join(root, ".codex", "rules", role+".md")

	for _, path := range []string{profilePath, rolePath, rulePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}
