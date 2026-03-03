package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotifyResetCompletionClearsMarker(t *testing.T) {
	dir := t.TempDir()
	trackerDir := filepath.Join(dir, "swarm")
	if err := os.MkdirAll(trackerDir, 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	configPath := filepath.Join(dir, "swarm.toml")
	config := `[project]
name = "test"
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
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	marker := filepath.Join(trackerDir, ".completion-notified")
	if err := os.WriteFile(marker, []byte("abc"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--config", configPath, "notify", "reset-completion"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker should be removed, stat err=%v", err)
	}
	if !strings.Contains(out.String(), "completion marker reset") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestNotifyResetCompletionNoopWhenMissing(t *testing.T) {
	dir := t.TempDir()
	trackerDir := filepath.Join(dir, "swarm")
	if err := os.MkdirAll(trackerDir, 0o755); err != nil {
		t.Fatalf("mkdir tracker dir: %v", err)
	}
	configPath := filepath.Join(dir, "swarm.toml")
	config := `[project]
name = "test"
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
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--config", configPath, "notify", "reset-completion"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "already clear") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
