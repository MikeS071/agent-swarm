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
	wantRepo := filepath.Clean(dir)
	if cfg.Project.Repo != wantRepo {
		t.Fatalf("expected resolved default repo %q, got %q", wantRepo, cfg.Project.Repo)
	}
	wantTracker := filepath.Join(wantRepo, def.Project.Tracker)
	if cfg.Project.Tracker != wantTracker {
		t.Fatalf("expected resolved default tracker %q, got %q", wantTracker, cfg.Project.Tracker)
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
	wantFeatures := filepath.Join(dir, def.Project.FeaturesDir)
	if cfg.Project.FeaturesDir != wantFeatures {
		t.Fatalf("expected resolved default features_dir %q, got %q", wantFeatures, cfg.Project.FeaturesDir)
	}
	wantProfiles := def.Profiles
	wantProfiles.Architect = filepath.Join(dir, def.Profiles.Architect)
	wantProfiles.CodeAgent = filepath.Join(dir, def.Profiles.CodeAgent)
	wantProfiles.TDDGuide = filepath.Join(dir, def.Profiles.TDDGuide)
	wantProfiles.CodeReviewer = filepath.Join(dir, def.Profiles.CodeReviewer)
	wantProfiles.SecurityReviewer = filepath.Join(dir, def.Profiles.SecurityReviewer)
	wantProfiles.E2ERunner = filepath.Join(dir, def.Profiles.E2ERunner)
	wantProfiles.DocUpdater = filepath.Join(dir, def.Profiles.DocUpdater)
	wantProfiles.RefactorCleaner = filepath.Join(dir, def.Profiles.RefactorCleaner)
	wantProfiles.BuildErrorResolver = filepath.Join(dir, def.Profiles.BuildErrorResolver)
	if !reflect.DeepEqual(cfg.Profiles, wantProfiles) {
		t.Fatalf("expected resolved default profiles config %+v, got %+v", wantProfiles, cfg.Profiles)
	}
	if !reflect.DeepEqual(cfg.PostBuild.Order, def.PostBuild.Order) {
		t.Fatalf("expected default post_build.order %v, got %v", def.PostBuild.Order, cfg.PostBuild.Order)
	}
	if !reflect.DeepEqual(cfg.PostBuild.ParallelGroups, def.PostBuild.ParallelGroups) {
		t.Fatalf("expected default post_build.parallel_groups %v, got %v", def.PostBuild.ParallelGroups, cfg.PostBuild.ParallelGroups)
	}
	wantLifecyclePolicy := filepath.Join(dir, def.Lifecycle.PolicyFile)
	if cfg.Lifecycle.PolicyFile != wantLifecyclePolicy {
		t.Fatalf("expected resolved lifecycle.policy_file %q, got %q", wantLifecyclePolicy, cfg.Lifecycle.PolicyFile)
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

[lifecycle]
policy_file = ".agents/custom-lifecycle-policy.toml"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantFeatures := filepath.Join(dir, "custom/features")
	if cfg.Project.FeaturesDir != wantFeatures {
		t.Fatalf("expected resolved features_dir %q, got %q", wantFeatures, cfg.Project.FeaturesDir)
	}
	wantProfile := filepath.Join(dir, "profiles/custom-code-agent.md")
	if cfg.Profiles.CodeAgent != wantProfile {
		t.Fatalf("expected resolved profiles.code_agent %q, got %q", wantProfile, cfg.Profiles.CodeAgent)
	}
	wantSecProfile := filepath.Join(dir, "profiles/custom-security.md")
	if cfg.Profiles.SecurityReviewer != wantSecProfile {
		t.Fatalf("expected resolved profiles.security_reviewer %q, got %q", wantSecProfile, cfg.Profiles.SecurityReviewer)
	}
	if !reflect.DeepEqual(cfg.PostBuild.Order, []string{"int", "review", "mem"}) {
		t.Fatalf("expected custom post_build.order, got %v", cfg.PostBuild.Order)
	}
	if !reflect.DeepEqual(cfg.PostBuild.ParallelGroups, [][]string{{"review", "sec"}}) {
		t.Fatalf("expected custom post_build.parallel_groups, got %v", cfg.PostBuild.ParallelGroups)
	}
	wantPolicy := filepath.Join(dir, ".agents/custom-lifecycle-policy.toml")
	if cfg.Lifecycle.PolicyFile != wantPolicy {
		t.Fatalf("expected resolved lifecycle.policy_file %q, got %q", wantPolicy, cfg.Lifecycle.PolicyFile)
	}
}

func TestLoadEmptyFeaturesDirDefaults(t *testing.T) {
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

	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error for empty project.features_dir: %v", err)
	}
	wantFeatures := filepath.Join(dir, "swarm/features")
	if got.Project.FeaturesDir != wantFeatures {
		t.Fatalf("features_dir=%q want %q", got.Project.FeaturesDir, wantFeatures)
	}
}

func TestLoadResolvesPathsRelativeToConfigDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := writeFile(t, dir, "swarm.toml", `
[project]
name = "myproject"
repo = "."
tracker = "swarm/tracker.json"
prompt_dir = "swarm/prompts"
features_dir = "swarm/features"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
`)

	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir("/")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Project.Repo != filepath.Clean(dir) {
		t.Fatalf("repo=%q want %q", cfg.Project.Repo, filepath.Clean(dir))
	}
	if cfg.Project.Tracker != filepath.Join(dir, "swarm/tracker.json") {
		t.Fatalf("tracker=%q", cfg.Project.Tracker)
	}
}

func TestLoadGuardianDefaults(t *testing.T) {
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

	if !cfg.Guardian.Enabled {
		t.Fatalf("expected guardian enabled by default")
	}
	wantFlow := filepath.Join(dir, "swarm/flow.v2.yaml")
	if cfg.Guardian.FlowFile != wantFlow {
		t.Fatalf("guardian.flow_file=%q want %q", cfg.Guardian.FlowFile, wantFlow)
	}
	if cfg.Guardian.Mode != "advisory" {
		t.Fatalf("guardian.mode=%q want advisory", cfg.Guardian.Mode)
	}
}

func TestLoadGuardianCustomConfig(t *testing.T) {
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

[guardian]
enabled = false
flow_file = "custom/flow.yaml"
mode = "ENFORCE"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Guardian.Enabled {
		t.Fatalf("expected guardian.enabled=false")
	}
	wantFlow := filepath.Join(dir, "custom/flow.yaml")
	if cfg.Guardian.FlowFile != wantFlow {
		t.Fatalf("guardian.flow_file=%q want %q", cfg.Guardian.FlowFile, wantFlow)
	}
	if cfg.Guardian.Mode != "enforce" {
		t.Fatalf("guardian.mode=%q want enforce", cfg.Guardian.Mode)
	}
}

func TestLoadGuardianInvalidMode(t *testing.T) {
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

[guardian]
mode = "strict"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected invalid guardian.mode error")
	}
}
