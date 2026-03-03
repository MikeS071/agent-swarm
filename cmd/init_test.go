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
