package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestFeatureCommandIsRegistered(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"feature", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("feature command should be available, got error: %v\noutput:\n%s", err, out.String())
	}
}

func TestFeatureLifecycleHappyPath(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)

	prdSrc := filepath.Join(repo, "prd-source.md")
	if err := os.WriteFile(prdSrc, []byte("# PRD\n"), 0o644); err != nil {
		t.Fatalf("write prd source: %v", err)
	}

	if _, err := runRootWithConfig(t, cfgPath, "feature", "add", "cache-overhaul", "--prd", prdSrc); err != nil {
		t.Fatalf("feature add: %v", err)
	}
	if _, err := runRootWithConfig(t, cfgPath, "feature", "approve-prd", "cache-overhaul", "--by", "mike"); err != nil {
		t.Fatalf("approve-prd: %v", err)
	}

	featureDir := filepath.Join(repo, "swarm", "features", "cache-overhaul")
	if err := os.WriteFile(filepath.Join(featureDir, "arch-review.md"), []byte("# Arch\n"), 0o644); err != nil {
		t.Fatalf("write arch review: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "spec.md"), []byte("# Spec\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	if _, err := runRootWithConfig(t, cfgPath, "feature", "arch-review", "cache-overhaul"); err != nil {
		t.Fatalf("arch-review: %v", err)
	}
	if _, err := runRootWithConfig(t, cfgPath, "feature", "approve-spec", "cache-overhaul", "--by", "mike"); err != nil {
		t.Fatalf("approve-spec: %v", err)
	}
	if _, err := runRootWithConfig(t, cfgPath, "feature", "plan", "cache-overhaul"); err != nil {
		t.Fatalf("plan: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "feature", "show", "cache-overhaul", "--json")
	if err != nil {
		t.Fatalf("show --json: %v", err)
	}
	var payload struct {
		Name           string `json:"name"`
		State          string `json:"state"`
		PRDApprovedBy  string `json:"prd_approved_by"`
		SpecApprovedBy string `json:"spec_approved_by"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal show output: %v\noutput: %s", err, out)
	}
	if payload.Name != "cache-overhaul" {
		t.Fatalf("name = %q, want %q", payload.Name, "cache-overhaul")
	}
	if payload.State != "building" {
		t.Fatalf("state = %q, want %q", payload.State, "building")
	}
	if payload.PRDApprovedBy != "mike" {
		t.Fatalf("prd_approved_by = %q, want %q", payload.PRDApprovedBy, "mike")
	}
	if payload.SpecApprovedBy != "mike" {
		t.Fatalf("spec_approved_by = %q, want %q", payload.SpecApprovedBy, "mike")
	}
}

func TestFeatureGateErrors(t *testing.T) {
	repo, cfgPath := setupFeatureTestProject(t)

	if _, err := runRootWithConfig(t, cfgPath, "feature", "add", "cache-overhaul"); err != nil {
		t.Fatalf("feature add: %v", err)
	}

	t.Run("approve-prd requires prd file", func(t *testing.T) {
		_, err := runRootWithConfig(t, cfgPath, "feature", "approve-prd", "cache-overhaul")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "prd.md") {
			t.Fatalf("expected prd gate error, got %v", err)
		}
	})

	t.Run("arch-review requires output file", func(t *testing.T) {
		featureDir := filepath.Join(repo, "swarm", "features", "cache-overhaul")
		if err := os.WriteFile(filepath.Join(featureDir, "prd.md"), []byte("# PRD\n"), 0o644); err != nil {
			t.Fatalf("write prd: %v", err)
		}
		if _, err := runRootWithConfig(t, cfgPath, "feature", "approve-prd", "cache-overhaul"); err != nil {
			t.Fatalf("approve-prd: %v", err)
		}

		_, err := runRootWithConfig(t, cfgPath, "feature", "arch-review", "cache-overhaul")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "arch-review.md") {
			t.Fatalf("expected arch review gate error, got %v", err)
		}
	})

	t.Run("complete requires post_build state", func(t *testing.T) {
		_, err := runRootWithConfig(t, cfgPath, "feature", "complete", "cache-overhaul")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "post_build") {
			t.Fatalf("expected post_build gate error, got %v", err)
		}
	})
}

func TestFeatureListSorted(t *testing.T) {
	_, cfgPath := setupFeatureTestProject(t)

	if _, err := runRootWithConfig(t, cfgPath, "feature", "add", "beta"); err != nil {
		t.Fatalf("add beta: %v", err)
	}
	if _, err := runRootWithConfig(t, cfgPath, "feature", "add", "alpha"); err != nil {
		t.Fatalf("add alpha: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "feature", "list")
	if err != nil {
		t.Fatalf("feature list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d (%q)", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "alpha\t") {
		t.Fatalf("first line should be alpha, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "beta\t") {
		t.Fatalf("second line should be beta, got %q", lines[1])
	}
}

func setupFeatureTestProject(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()

	cfgPath := filepath.Join(repo, "swarm.toml")
	cfg := `
[project]
name = "agent-swarm"
repo = "."
base_branch = "main"
max_agents = 3
min_ram_mb = 256
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
features_dir = "swarm/features"

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
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "swarm", "features"), 0o755); err != nil {
		t.Fatalf("mkdir features: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{"project":"agent-swarm","tickets":{}}`)

	return repo, cfgPath
}

func runRootWithConfig(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"--config", cfgPath}, args...)
	resetCommandFlags(rootCmd)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(fullArgs)
	err := rootCmd.Execute()
	return strings.TrimSpace(out.String()), err
}

func resetCommandFlags(cmd *cobra.Command) {
	resetFlagSet := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			_ = fs.Set(f.Name, f.DefValue)
			f.Changed = false
		})
	}
	resetFlagSet(cmd.PersistentFlags())
	resetFlagSet(cmd.Flags())
	for _, sub := range cmd.Commands() {
		resetCommandFlags(sub)
	}
}
