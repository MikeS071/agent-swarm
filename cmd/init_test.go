package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldProject_ArchivesLegacyWorkflowFiles(t *testing.T) {
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

func TestScaffoldProject_DoesNotCreateLegacyArchiveWhenNoLegacyFiles(t *testing.T) {
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

func TestScaffoldProject_ErrorsWhenLegacyArchivePathIsBlocked(t *testing.T) {
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
