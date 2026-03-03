package cmd

import (
	"os"
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

	if _, err := os.Stat(filepath.Join(root, "swarm", "tracker.json")); err != nil {
		t.Fatalf("expected tracker.json to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}

	cfg, err := config.Load(filepath.Join(root, "swarm.toml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Project.Name != "my-project" {
		t.Fatalf("project name = %q, want %q", cfg.Project.Name, "my-project")
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
