package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
)

func setupGuardianMigrateProject(t *testing.T, withGuardian bool) (repo, cfgPath string) {
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

	guardianBlock := ""
	if withGuardian {
		guardianBlock = `
[guardian]
enabled = false
mode = "enforce"
flow_file = "legacy/guardian-flow.yaml"
`
	}

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
` + guardianBlock
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return repo, cfgPath
}

func TestGuardianMigrateDryRunReportsPlannedChanges(t *testing.T) {
	repo, cfgPath := setupGuardianMigrateProject(t, false)

	out, err := runRootWithConfig(t, cfgPath, "guardian", "migrate")
	if err != nil {
		t.Fatalf("guardian migrate dry-run: %v", err)
	}
	if !strings.Contains(out, "guardian migrate (dry-run)") {
		t.Fatalf("expected dry-run header, got: %s", out)
	}
	if !strings.Contains(out, "set guardian.enabled=true") {
		t.Fatalf("expected enabled change in output: %s", out)
	}
	if !strings.Contains(out, "create swarm/flow.v2.yaml") {
		t.Fatalf("expected flow scaffold change in output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(repo, "swarm", "flow.v2.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create flow file")
	}
}

func TestGuardianMigrateApplyWritesSafeDefaults(t *testing.T) {
	repo, cfgPath := setupGuardianMigrateProject(t, true)

	out, err := runRootWithConfig(t, cfgPath, "guardian", "migrate", "--apply")
	if err != nil {
		t.Fatalf("guardian migrate --apply: %v", err)
	}
	if !strings.Contains(out, "guardian migrate (applied)") {
		t.Fatalf("expected applied header, got: %s", out)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Guardian.Enabled {
		t.Fatalf("guardian.enabled expected true after migrate")
	}
	if cfg.Guardian.Mode != "advisory" {
		t.Fatalf("guardian.mode=%q want advisory", cfg.Guardian.Mode)
	}
	wantFlow := filepath.Join(repo, "swarm", "flow.v2.yaml")
	if cfg.Guardian.FlowFile != wantFlow {
		t.Fatalf("guardian.flow_file=%q want %q", cfg.Guardian.FlowFile, wantFlow)
	}
	b, err := os.ReadFile(wantFlow)
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if !strings.Contains(string(b), "version: 2") {
		t.Fatalf("expected flow scaffold contents in %s", wantFlow)
	}
}

func TestGuardianMigrateApplyIsIdempotent(t *testing.T) {
	_, cfgPath := setupGuardianMigrateProject(t, false)

	if _, err := runRootWithConfig(t, cfgPath, "guardian", "migrate", "--apply"); err != nil {
		t.Fatalf("first migrate apply failed: %v", err)
	}
	out, err := runRootWithConfig(t, cfgPath, "guardian", "migrate", "--apply")
	if err != nil {
		t.Fatalf("second migrate apply failed: %v", err)
	}
	if !strings.Contains(out, "0 change(s)") {
		t.Fatalf("expected idempotent second apply, got: %s", out)
	}
}
