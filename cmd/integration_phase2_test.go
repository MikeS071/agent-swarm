package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/roles"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

// --- helpers ---

func setupPhase2Project(t *testing.T) (root string, cfgPath string) {
	t.Helper()
	root = t.TempDir()
	cfgPath = filepath.Join(root, "swarm.toml")

	cfg := `[project]
name = "test-project"
repo = "` + root + `"
base_branch = "main"
max_agents = 1
min_ram_mb = 128
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
require_explicit_role = true
require_verify_cmd = true

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
interval = "5m"
`
	writePath(t, cfgPath, cfg)
	writePath(t, filepath.Join(root, "swarm", "prompts", ".keep"), "")
	return root, cfgPath
}

func writeTrackerStruct(t *testing.T, root string, tickets map[string]tracker.Ticket) {
	t.Helper()
	tr := tracker.Tracker{
		Project: "test-project",
		Tickets: tickets,
	}
	b, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writePath(t, filepath.Join(root, "swarm", "tracker.json"), string(b))
}

func setupRoleAssets(t *testing.T, root, role string) {
	t.Helper()
	// Profile at .agents/profiles/<role>.md
	writePath(t, filepath.Join(root, ".agents", "profiles", role+".md"), "# Profile: "+role+"\nYou are a specialist.\n")
	// Role spec at .agents/roles/<role>.yaml
	writePath(t, filepath.Join(root, ".agents", "roles", role+".yaml"), "name: "+role+"\ntype: specialist\n")
	// Role rule at .codex/rules/<role>.md
	writePath(t, filepath.Join(root, ".codex", "rules", role+".md"), "# Rules for "+role+"\nFollow TDD.\n")
	// Base rules at .codex/rules/common/
	for _, f := range roles.RequiredBaseRuleFiles() {
		writePath(t, filepath.Join(root, ".codex", "rules", "common", f), "# "+f+"\nStandard rule.\n")
	}
}

// --- integration tests ---

func TestPhase2_PrepPassesWithValidRoleAndPrompt(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)
	setupRoleAssets(t, root, "backend")

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\nImplement the thing.\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err != nil {
		t.Fatalf("prep should pass with valid role + prompt, error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "prep ok") {
		t.Fatalf("expected 'prep ok' in output, got:\n%s", out)
	}
}

func TestPhase2_PrepPassesWithoutRoleAssets(t *testing.T) {
	// Prep validates ticket metadata (profile field, verify_cmd, prompt file)
	// but does NOT validate role filesystem assets — that is roles check's job.
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err != nil {
		t.Fatalf("prep should pass with valid ticket metadata even without role assets, error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "prep ok") {
		t.Fatalf("expected prep ok, got:\n%s", out)
	}
}

func TestPhase2_RolesCheckFailsMissingRoleAssets(t *testing.T) {
	// roles check should fail when role assets are missing on disk
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})

	out, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err == nil {
		t.Fatalf("roles check should fail when role assets missing, got success:\n%s", out)
	}
	var payload struct {
		OK       bool            `json:"ok"`
		Failures []roles.Failure `json:"failures"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("unmarshal roles check json: %v\noutput:\n%s", jsonErr, out)
	}
	if payload.OK {
		t.Fatalf("expected ok=false when role assets missing")
	}
}

func TestPhase2_PrepFailsWhenProfileFieldEmpty(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "",
			VerifyCmd: "go test ./...",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err == nil {
		t.Fatalf("prep should fail when profile empty, got success:\n%s", out)
	}
	if !strings.Contains(out, "profile") || !strings.Contains(out, "missing") {
		t.Fatalf("expected actionable output about missing profile, got:\n%s", out)
	}
}

func TestPhase2_PrepFailsWhenVerifyCmdEmpty(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err == nil {
		t.Fatalf("prep should fail when verify_cmd empty, got success:\n%s", out)
	}
	if !strings.Contains(out, "verify_cmd") || !strings.Contains(out, "missing") {
		t.Fatalf("expected actionable output about missing verify_cmd, got:\n%s", out)
	}
}

func TestPhase2_RolesCheckPassesWithValidAssets(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)
	setupRoleAssets(t, root, "backend")

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})

	out, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err != nil {
		t.Fatalf("roles check should pass with valid assets, error: %v\noutput:\n%s", err, out)
	}

	var payload struct {
		OK       bool            `json:"ok"`
		Failures []roles.Failure `json:"failures"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal roles check json: %v\noutput:\n%s", err, out)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true, got failures: %+v", payload.Failures)
	}
}

func TestPhase2_RolesCheckFailsMissingBaseRules(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)
	// Create role-specific assets but NOT base rules
	writePath(t, filepath.Join(root, ".agents", "profiles", "backend.md"), "# Profile\n")
	writePath(t, filepath.Join(root, ".agents", "roles", "backend.yaml"), "name: backend\n")
	writePath(t, filepath.Join(root, ".codex", "rules", "backend.md"), "# Rules\n")

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "test ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})

	out, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err == nil {
		t.Fatalf("roles check should fail missing base rules, got success:\n%s", out)
	}

	var payload struct {
		OK       bool            `json:"ok"`
		Failures []roles.Failure `json:"failures"`
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("unmarshal roles check json: %v\noutput:\n%s", jsonErr, out)
	}
	if payload.OK {
		t.Fatalf("expected ok=false")
	}
	hasBaseRule := false
	for _, f := range payload.Failures {
		if f.Asset == roles.AssetBaseRule {
			hasBaseRule = true
			break
		}
	}
	if !hasBaseRule {
		t.Fatalf("expected at least one base_rule failure, got: %+v", payload.Failures)
	}
}

func TestPhase2_WatchGatedByPrepFailure(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:  tracker.StatusTodo,
			Phase:   1,
			Branch:  "feat/t-01",
			Desc:    "test ticket",
			Profile: "",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "watch", "--once")
	if err == nil {
		t.Fatalf("watch should be gated by prep failure, got success:\n%s", out)
	}
	if !strings.Contains(err.Error(), "prep gate failed") {
		t.Fatalf("expected 'prep gate failed' error, got: %v", err)
	}
}

func TestPhase2_WatchAllowUnpreparedBypassesPrepGate(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:  tracker.StatusTodo,
			Phase:   1,
			Branch:  "feat/t-01",
			Desc:    "test ticket",
			Profile: "",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "watch", "--once", "--dry-run", "--allow-unprepared")
	if err != nil {
		t.Fatalf("watch --allow-unprepared should bypass prep gate, error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "WARNING") {
		t.Fatalf("expected WARNING about unprepared state, got:\n%s", out)
	}
}

func TestPhase2_ContextSnapshotMaterializesManifest(t *testing.T) {
	root := t.TempDir()
	setupRoleAssets(t, root, "backend")

	wt := worktree.New(root, "", "main")

	worktreeDir := filepath.Join(root, "worktrees", "t-01")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := worktree.AgentContextOptions{
		TicketID:     "t-01",
		WorktreePath: worktreeDir,
		Role:         "backend",
		RunID:        "run-001",
	}
	manifestPath, manifest, err := wt.MaterializeAgentContext(opts)
	if err != nil {
		t.Fatalf("materialize agent context failed: %v", err)
	}

	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest file not found at %s: %v", manifestPath, err)
	}
	if manifest.Ticket != "t-01" {
		t.Fatalf("expected manifest ticket=t-01, got=%s", manifest.Ticket)
	}
	if manifest.Role != "backend" {
		t.Fatalf("expected manifest role=backend, got=%s", manifest.Role)
	}
	if manifest.RunID != "run-001" {
		t.Fatalf("expected manifest runId=run-001, got=%s", manifest.RunID)
	}
	if len(manifest.Sources) == 0 {
		t.Fatalf("expected at least one source in manifest, got 0")
	}
	for _, src := range manifest.Sources {
		if src.SHA256 == "" {
			t.Fatalf("source %s has empty SHA256", src.Path)
		}
		if len(src.SHA256) != 64 {
			t.Fatalf("source %s has invalid SHA256 length: %d", src.Path, len(src.SHA256))
		}
	}

	agentCtxDir := filepath.Join(worktreeDir, ".agent-context")
	if _, err := os.Stat(agentCtxDir); err != nil {
		t.Fatalf(".agent-context dir not found: %v", err)
	}
}

func TestPhase2_ContextSnapshotIsDeterministic(t *testing.T) {
	root := t.TempDir()
	setupRoleAssets(t, root, "backend")

	wt := worktree.New(root, "", "main")

	var manifests [2]worktree.ContextManifest
	for i := 0; i < 2; i++ {
		dir := filepath.Join(root, "worktrees", "t-01", strings.Repeat("x", i+1))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, m, err := wt.MaterializeAgentContext(worktree.AgentContextOptions{
			TicketID:     "t-01",
			WorktreePath: dir,
			Role:         "backend",
			RunID:        "run-det",
		})
		if err != nil {
			t.Fatalf("run %d: materialize failed: %v", i, err)
		}
		manifests[i] = m
	}

	if len(manifests[0].Sources) != len(manifests[1].Sources) {
		t.Fatalf("determinism: source counts differ (%d vs %d)",
			len(manifests[0].Sources), len(manifests[1].Sources))
	}
	for i, s0 := range manifests[0].Sources {
		s1 := manifests[1].Sources[i]
		if s0.SHA256 != s1.SHA256 {
			t.Fatalf("determinism: hash mismatch for %s (%s vs %s)", s0.Path, s0.SHA256, s1.SHA256)
		}
	}
}

func TestPhase2_EndToEnd_PrepRolesContextPipeline(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)
	setupRoleAssets(t, root, "backend")

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "build feature",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\nBuild the feature.\n")

	// Step 1: prep
	prepOut, err := runRootWithConfig(t, cfgPath, "prep")
	if err != nil {
		t.Fatalf("step 1 (prep): unexpected failure: %v\noutput:\n%s", err, prepOut)
	}

	// Step 2: roles check
	rolesOut, err := runRootWithConfig(t, cfgPath, "roles", "check", "--json")
	if err != nil {
		t.Fatalf("step 2 (roles check): unexpected failure: %v\noutput:\n%s", err, rolesOut)
	}
	var rolesPayload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(rolesOut), &rolesPayload); err != nil {
		t.Fatalf("step 2: unmarshal roles json: %v", err)
	}
	if !rolesPayload.OK {
		t.Fatalf("step 2: expected roles ok=true, got:\n%s", rolesOut)
	}

	// Step 3: context snapshot
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	trackerPath := resolveFromConfig(cfgPath, cfg.Project.Tracker)
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		t.Fatal(err)
	}

	wt := worktree.New(cfg.Project.Repo, "", cfg.Project.BaseBranch)
	worktreeDir := filepath.Join(root, "worktrees", "t-01")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ticket := tr.Tickets["t-01"]
	_, manifest, err := wt.MaterializeAgentContext(worktree.AgentContextOptions{
		TicketID:     "t-01",
		WorktreePath: worktreeDir,
		Role:         ticket.Profile,
		RunID:        "integration-test-run",
	})
	if err != nil {
		t.Fatalf("step 3 (context snapshot): failed: %v", err)
	}

	if manifest.Ticket != "t-01" {
		t.Fatalf("manifest ticket mismatch: got %s", manifest.Ticket)
	}
	if manifest.Role != "backend" {
		t.Fatalf("manifest role mismatch: got %s", manifest.Role)
	}
	if len(manifest.Sources) == 0 {
		t.Fatalf("manifest has no sources")
	}
}

func TestPhase2_DoneTicketsSkippedByPrepPipeline(t *testing.T) {
	root, cfgPath := setupPhase2Project(t)
	setupRoleAssets(t, root, "backend")

	writeTrackerStruct(t, root, map[string]tracker.Ticket{
		"t-done": {
			Status:    tracker.StatusDone,
			Phase:     1,
			Branch:    "feat/t-done",
			Desc:      "already done",
			Profile:   "",
			VerifyCmd: "",
		},
		"t-01": {
			Status:    tracker.StatusTodo,
			Phase:     1,
			Branch:    "feat/t-01",
			Desc:      "active ticket",
			Profile:   "backend",
			VerifyCmd: "go test ./...",
		},
	})
	writePath(t, filepath.Join(root, "swarm", "prompts", "t-01.md"), "# t-01\n")

	out, err := runRootWithConfig(t, cfgPath, "prep")
	if err != nil {
		t.Fatalf("prep should skip done tickets, error: %v\noutput:\n%s", err, out)
	}
}
