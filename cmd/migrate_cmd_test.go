package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateCommandIsRegistered(t *testing.T) {
	out, err := runRootWithConfig(t, writeMigrateConfig(t), "migrate", "--help")
	if err != nil {
		t.Fatalf("migrate command should be available, got error: %v\noutput:\n%s", err, out)
	}
}

func TestMigratePostBuildRunScopeFlagValidation(t *testing.T) {
	cfgPath := writeMigrateConfig(t)

	_, err := runRootWithConfig(t, cfgPath, "migrate", "post-build-run-scope")
	if err == nil {
		t.Fatalf("expected error when neither --dry-run nor --apply is set")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = runRootWithConfig(t, cfgPath, "migrate", "post-build-run-scope", "--dry-run", "--apply")
	if err == nil {
		t.Fatalf("expected error when both flags are set")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigratePostBuildRunScopeDryRunAndApply(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeMigrateConfigAt(t, dir)
	trackerPath := filepath.Join(dir, "swarm", "tracker.json")
	writeMigrateTracker(t, trackerPath)

	before, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("read tracker before dry-run: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "migrate", "post-build-run-scope", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run command failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "dry-run") {
		t.Fatalf("expected dry-run output, got:\n%s", out)
	}

	afterDryRun, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("read tracker after dry-run: %v", err)
	}
	if string(before) != string(afterDryRun) {
		t.Fatalf("dry-run mutated tracker")
	}

	out, err = runRootWithConfig(t, cfgPath, "migrate", "post-build-run-scope", "--apply")
	if err != nil {
		t.Fatalf("apply command failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "applied") {
		t.Fatalf("expected apply output, got:\n%s", out)
	}

	afterApply, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("read tracker after apply: %v", err)
	}
	text := string(afterApply)
	if strings.Contains(text, "\"int-a\"") {
		t.Fatalf("expected legacy int-a ticket removed after apply")
	}
	if !strings.Contains(text, "\"currentRunId\": \"legacy-run-1\"") {
		t.Fatalf("expected currentRunId in transformed tracker")
	}
}

func writeMigrateConfig(t *testing.T) string {
	t.Helper()
	return writeMigrateConfigAt(t, t.TempDir())
}

func writeMigrateConfigAt(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "swarm"), 0o755); err != nil {
		t.Fatalf("mkdir swarm dir: %v", err)
	}
	cfgPath := filepath.Join(root, "swarm.toml")
	cfg := `[project]
name = "proj"
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

[post_build]
order = ["int", "gap", "tst"]
parallel_groups = []
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

func writeMigrateTracker(t *testing.T, trackerPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(trackerPath), 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	contents := `{
  "project": "proj",
  "tickets": {
    "feat-a-01": {"status": "done", "phase": 1, "depends": [], "type": "build", "feature": "a"},
    "int-a": {"status": "done", "phase": 2, "depends": ["feat-a-01"], "type": "int", "feature": "a"},
    "gap-a": {"status": "done", "phase": 2, "depends": ["int-a"], "type": "gap", "feature": "a"},
    "tst-a": {"status": "done", "phase": 2, "depends": ["int-a"], "type": "tst", "feature": "a"},
    "deploy": {"status": "todo", "phase": 3, "depends": ["gap-a", "tst-a"], "type": "feature"}
  }
}`
	if err := os.WriteFile(trackerPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
}
