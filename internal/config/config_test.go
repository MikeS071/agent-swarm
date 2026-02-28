package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return p
}

func TestLoadValidConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, "swarm.toml", `
[project]
name = "myproject"
repo = "."
base_branch = "main"
max_agents = 7
min_ram_mb = 1024
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
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Project.Name != "myproject" {
		t.Fatalf("expected project name myproject, got %q", cfg.Project.Name)
	}
	if cfg.Project.MaxAgents != 7 {
		t.Fatalf("expected max_agents 7, got %d", cfg.Project.MaxAgents)
	}
	if cfg.Backend.Type != "codex-tmux" {
		t.Fatalf("expected backend type codex-tmux, got %q", cfg.Backend.Type)
	}
	if cfg.Watchdog.MaxRetries != 2 {
		t.Fatalf("expected max_retries 2, got %d", cfg.Watchdog.MaxRetries)
	}
}

func TestLoadAppliesDefaultsForMissingOptionalFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, "swarm.toml", `
[project]
name = "myproject"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	def := Default()
	if cfg.Project.Repo != def.Project.Repo {
		t.Fatalf("expected default repo %q, got %q", def.Project.Repo, cfg.Project.Repo)
	}
	if cfg.Project.Tracker != def.Project.Tracker {
		t.Fatalf("expected default tracker %q, got %q", def.Project.Tracker, cfg.Project.Tracker)
	}
	if cfg.Backend.Model != def.Backend.Model {
		t.Fatalf("expected default model %q, got %q", def.Backend.Model, cfg.Backend.Model)
	}
	if cfg.Watchdog.Interval != def.Watchdog.Interval {
		t.Fatalf("expected default interval %q, got %q", def.Watchdog.Interval, cfg.Watchdog.Interval)
	}
}

func TestLoadMissingRequiredFieldFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, "swarm.toml", `
[project]
repo = "."

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing required project.name")
	}
}
