package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestLoadDefaultsNewConfigSections(t *testing.T) {
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
	if cfg.Project.FeaturesDir != def.Project.FeaturesDir {
		t.Fatalf("expected default features_dir %q, got %q", def.Project.FeaturesDir, cfg.Project.FeaturesDir)
	}
	if !reflect.DeepEqual(cfg.Profiles, def.Profiles) {
		t.Fatalf("expected default profiles config %+v, got %+v", def.Profiles, cfg.Profiles)
	}
	if !reflect.DeepEqual(cfg.PostBuild.Order, def.PostBuild.Order) {
		t.Fatalf("expected default post_build.order %v, got %v", def.PostBuild.Order, cfg.PostBuild.Order)
	}
	if !reflect.DeepEqual(cfg.PostBuild.ParallelGroups, def.PostBuild.ParallelGroups) {
		t.Fatalf("expected default post_build.parallel_groups %v, got %v", def.PostBuild.ParallelGroups, cfg.PostBuild.ParallelGroups)
	}
}

func TestLoadParsesCustomProfilesAndPostBuild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, "swarm.toml", `
[project]
name = "myproject"
features_dir = "custom/features"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]

[profiles]
code_agent = "profiles/custom-code-agent.md"
security_reviewer = "profiles/custom-security.md"

[post_build]
order = ["int", "review", "mem"]
parallel_groups = [["review", "sec"]]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Project.FeaturesDir != "custom/features" {
		t.Fatalf("expected features_dir custom/features, got %q", cfg.Project.FeaturesDir)
	}
	if cfg.Profiles.CodeAgent != "profiles/custom-code-agent.md" {
		t.Fatalf("expected profiles.code_agent override, got %q", cfg.Profiles.CodeAgent)
	}
	if cfg.Profiles.SecurityReviewer != "profiles/custom-security.md" {
		t.Fatalf("expected profiles.security_reviewer override, got %q", cfg.Profiles.SecurityReviewer)
	}
	if !reflect.DeepEqual(cfg.PostBuild.Order, []string{"int", "review", "mem"}) {
		t.Fatalf("expected custom post_build.order, got %v", cfg.PostBuild.Order)
	}
	if !reflect.DeepEqual(cfg.PostBuild.ParallelGroups, [][]string{{"review", "sec"}}) {
		t.Fatalf("expected custom post_build.parallel_groups, got %v", cfg.PostBuild.ParallelGroups)
	}
}

func TestLoadEmptyFeaturesDirFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeFile(t, dir, "swarm.toml", `
[project]
name = "myproject"
features_dir = "   "

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty project.features_dir")
	}
}
