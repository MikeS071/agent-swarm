package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
)

func TestScaffoldProjectCreatesExpectedLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my-project")

	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject() error = %v", err)
	}

	requiredDirs := []string{
		filepath.Join(root, "swarm", "prompts"),
		filepath.Join(root, "swarm", "features"),
		filepath.Join(root, "swarm", "logs"),
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".agents", "profiles"),
		filepath.Join(root, ".codex", "rules"),
	}
	for _, dir := range requiredDirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s to exist", dir)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "swarm", "tracker.seed.json")); err != nil {
		t.Fatalf("expected tracker.seed.json to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "swarm", "flow.v2.yaml")); err != nil {
		t.Fatalf("expected swarm/flow.v2.yaml to exist: %v", err)
	}

	cfg, err := config.Load(filepath.Join(root, "swarm.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Project.Name != "my-project" {
		t.Fatalf("project name = %q, want %q", cfg.Project.Name, "my-project")
	}
	wantFlow := filepath.Join(root, "swarm", "flow.v2.yaml")
	if cfg.Guardian.FlowFile != wantFlow {
		t.Fatalf("guardian.flow_file = %q, want %q", cfg.Guardian.FlowFile, wantFlow)
	}
	if _, err := os.Stat(cfg.Project.Tracker); err != nil {
		t.Fatalf("expected state tracker to exist at %s: %v", cfg.Project.Tracker, err)
	}
}

func TestInitScaffoldCreatesGuardianFlowFile(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "init-guardian-flow")

	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject() error = %v", err)
	}

	flowPath := filepath.Join(root, "swarm", "flow.v2.yaml")
	b, err := os.ReadFile(flowPath)
	if err != nil {
		t.Fatalf("read flow.v2.yaml: %v", err)
	}
	if !strings.Contains(string(b), "version: 2") {
		t.Fatalf("expected flow.v2.yaml to contain version header")
	}

	cfg, err := config.Load(filepath.Join(root, "swarm.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Guardian.FlowFile != flowPath {
		t.Fatalf("guardian.flow_file = %q, want %q", cfg.Guardian.FlowFile, flowPath)
	}
}

func TestScaffoldProjectFailsWhenConfigAlreadyExists(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "swarm.toml"), []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("write swarm.toml: %v", err)
	}

	err := scaffoldProject(root)
	if err == nil {
		t.Fatalf("expected error when swarm.toml already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "already exists")
	}
}

func TestScaffoldProjectDotPathUsesCurrentDirectoryName(t *testing.T) {
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(prev)
	}()

	if err := scaffoldProject("."); err != nil {
		t.Fatalf("scaffoldProject('.') error = %v", err)
	}

	cfg, err := config.Load(filepath.Join(root, "swarm.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := filepath.Base(root)
	if cfg.Project.Name != want {
		t.Fatalf("project name = %q, want %q", cfg.Project.Name, want)
	}
}

func TestScaffoldProjectArchivesLegacyWorkflowFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workflowPath := filepath.Join(root, "WORKFLOW_AUTO.md")
	sprintPath := filepath.Join(root, "sprint.json")

	if err := os.WriteFile(workflowPath, []byte("legacy workflow\n"), 0o644); err != nil {
		t.Fatalf("write WORKFLOW_AUTO.md: %v", err)
	}
	if err := os.WriteFile(sprintPath, []byte("{\"legacy\":true}\n"), 0o644); err != nil {
		t.Fatalf("write sprint.json: %v", err)
	}

	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}

	if _, err := os.Stat(workflowPath); !os.IsNotExist(err) {
		t.Fatalf("WORKFLOW_AUTO.md should be archived from project root")
	}
	if _, err := os.Stat(sprintPath); !os.IsNotExist(err) {
		t.Fatalf("sprint.json should be archived from project root")
	}

	archivedWorkflow := filepath.Join(root, "swarm", "archive", "legacy-workflow", "WORKFLOW_AUTO.md")
	archivedSprint := filepath.Join(root, "swarm", "archive", "legacy-workflow", "sprint.json")

	workflowData, err := os.ReadFile(archivedWorkflow)
	if err != nil {
		t.Fatalf("read archived WORKFLOW_AUTO.md: %v", err)
	}
	if string(workflowData) != "legacy workflow\n" {
		t.Fatalf("unexpected archived workflow contents: %q", string(workflowData))
	}

	sprintData, err := os.ReadFile(archivedSprint)
	if err != nil {
		t.Fatalf("read archived sprint.json: %v", err)
	}
	if string(sprintData) != "{\"legacy\":true}\n" {
		t.Fatalf("unexpected archived sprint contents: %q", string(sprintData))
	}
}

func TestScaffoldProjectDoesNotCreateLegacyArchiveWhenNoLegacyFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}

	archiveDir := filepath.Join(root, "swarm", "archive", "legacy-workflow")
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Fatalf("legacy archive directory should not exist when no legacy files are present")
	}
}

func TestScaffoldProjectErrorsWhenLegacyArchivePathIsBlocked(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "swarm"), 0o755); err != nil {
		t.Fatalf("mkdir swarm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "swarm", "archive"), []byte("not-a-dir\n"), 0o644); err != nil {
		t.Fatalf("create blocking archive file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "WORKFLOW_AUTO.md"), []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("write WORKFLOW_AUTO.md: %v", err)
	}

	err := scaffoldProject(root)
	if err == nil {
		t.Fatalf("expected scaffoldProject to fail when legacy archive path is blocked")
	}
	if !strings.Contains(err.Error(), "archive legacy workflow files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScaffoldProjectRegistersProjectInOpenClawRegistry(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "my-reg-project")
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	if err := os.Setenv("OPENCLAW_PROJECTS_REGISTRY", registryPath); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("OPENCLAW_PROJECTS_REGISTRY") })

	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}

	b, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var reg map[string]map[string]any
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatalf("parse registry: %v", err)
	}
	entry, ok := reg["my-reg-project"]
	if !ok {
		t.Fatalf("expected my-reg-project in registry")
	}
	expectedTrackerSuffix := filepath.Join(".local", "state", "agent-swarm", "projects", "my-reg-project", "tracker.json")
	if got, _ := entry["tracker"].(string); !strings.HasSuffix(got, expectedTrackerSuffix) {
		t.Fatalf("tracker=%q want suffix %q", got, expectedTrackerSuffix)
	}
	if got, _ := entry["promptDir"].(string); got != filepath.Join(root, "swarm", "prompts") {
		t.Fatalf("promptDir=%q want %q", got, filepath.Join(root, "swarm", "prompts"))
	}
}

func TestScaffoldProjectBootstrapsGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := filepath.Join(t.TempDir(), "git-bootstrap-project")
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	if err := os.Setenv("OPENCLAW_PROJECTS_REGISTRY", registryPath); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("OPENCLAW_PROJECTS_REGISTRY") })

	if err := scaffoldProject(root); err != nil {
		t.Fatalf("scaffoldProject: %v", err)
	}

	cmd := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected git repo, err=%v out=%s", err, string(out))
	}
	cmd = exec.Command("git", "-C", root, "rev-parse", "--verify", "refs/heads/main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected main branch, err=%v out=%s", err, string(out))
	}
	cmd = exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("expected initial commit, err=%v out=%s", err, string(out))
	}
}
