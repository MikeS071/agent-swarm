package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPhase1FlowOrderAndFailures(t *testing.T) {
	tests := []struct {
		name         string
		failAt       string
		wantErrPart  string
		wantExecuted []string
	}{
		{
			name:         "happy path executes all steps in order",
			wantExecuted: []string{"tickets lint", "prompts build --all", "prompts validate --strict"},
		},
		{
			name:         "stops on lint failure",
			failAt:       "tickets lint",
			wantErrPart:  "phase1 pipeline failed at tickets lint",
			wantExecuted: []string{"tickets lint"},
		},
		{
			name:         "stops on build failure",
			failAt:       "prompts build --all",
			wantErrPart:  "phase1 pipeline failed at prompts build --all",
			wantExecuted: []string{"tickets lint", "prompts build --all"},
		},
		{
			name:         "stops on validate failure",
			failAt:       "prompts validate --strict",
			wantErrPart:  "phase1 pipeline failed at prompts validate --strict",
			wantExecuted: []string{"tickets lint", "prompts build --all", "prompts validate --strict"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed := make([]string, 0, 3)
			steps := []phase1Step{
				{
					Name: "tickets lint",
					Run: func() error {
						executed = append(executed, "tickets lint")
						if tt.failAt == "tickets lint" {
							return errors.New("lint failed")
						}
						return nil
					},
				},
				{
					Name: "prompts build --all",
					Run: func() error {
						executed = append(executed, "prompts build --all")
						if tt.failAt == "prompts build --all" {
							return errors.New("build failed")
						}
						return nil
					},
				},
				{
					Name: "prompts validate --strict",
					Run: func() error {
						executed = append(executed, "prompts validate --strict")
						if tt.failAt == "prompts validate --strict" {
							return errors.New("validate failed")
						}
						return nil
					},
				},
			}

			var out bytes.Buffer
			err := runPhase1Flow(&out, steps)
			if tt.wantErrPart == "" {
				if err != nil {
					t.Fatalf("runPhase1Flow() error = %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("runPhase1Flow() expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrPart)
				}
			}

			if strings.Join(executed, ",") != strings.Join(tt.wantExecuted, ",") {
				t.Fatalf("executed steps = %#v, want %#v", executed, tt.wantExecuted)
			}
		})
	}
}

func TestPrepCommandRunsPhase1PipelineHappyPath(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePrepConfig(t, repo)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	writePrepFile(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "tp-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/tp-01",
      "desc": "schema"
    },
    "tp-02": {
      "status": "todo",
      "phase": 1,
      "depends": ["tp-01"],
      "branch": "feat/tp-02",
      "desc": "lint"
    }
  }
}`)

	out, err := executeRoot(t, "--config", cfgPath, "prep", "--strict")
	if err != nil {
		t.Fatalf("prep failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tickets lint") || !strings.Contains(out, "prompts build --all") || !strings.Contains(out, "prompts validate --strict") {
		t.Fatalf("prep output missing pipeline steps:\n%s", out)
	}

	for _, ticketID := range []string{"tp-01", "tp-02"} {
		if _, statErr := os.Stat(filepath.Join(repo, "swarm", "prompts", ticketID+".md")); statErr != nil {
			t.Fatalf("expected prompt for %s: %v", ticketID, statErr)
		}
	}
}

func TestPrepCommandFailsWithClearStepMessage(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePrepConfig(t, repo)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	writePrepFile(t, trackerPath, "{ not-json }")

	out, err := executeRoot(t, "--config", cfgPath, "prep", "--strict")
	if err == nil {
		t.Fatalf("expected prep to fail\n%s", out)
	}
	if !strings.Contains(err.Error(), "phase1 pipeline failed at tickets lint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepCommandFailsValidateOnPlaceholderPrompt(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePrepConfig(t, repo)
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	writePrepFile(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "tp-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/tp-01",
      "desc": "schema"
    }
  }
}`)
	writePrepFile(t, filepath.Join(repo, "swarm", "prompts", "bad.md"), "# BAD\n\nTODO: fill this\n")

	out, err := executeRoot(t, "--config", cfgPath, "prep", "--strict")
	if err == nil {
		t.Fatalf("expected prep to fail in validate step\n%s", out)
	}
	if !strings.Contains(err.Error(), "phase1 pipeline failed at prompts validate --strict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func writePrepConfig(t *testing.T, repo string) string {
	t.Helper()
	cfgPath := filepath.Join(repo, "swarm.toml")
	writePrepFile(t, cfgPath, `[project]
name = "agent-swarm"
repo = "."
base_branch = "main"
max_agents = 3
min_ram_mb = 128
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"

[backend]
type = "codex-tmux"

[notifications]
type = "stdout"

[watchdog]
interval = "5m"
`)
	return cfgPath
}

func writePrepFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
