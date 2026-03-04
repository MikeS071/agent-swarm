package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type ticketLintJSON struct {
	Strict     bool `json:"strict"`
	TicketCount int  `json:"ticket_count"`
	IssueCount int  `json:"issue_count"`
	Issues     []struct {
		TicketID string `json:"ticket_id"`
		Path     string `json:"path"`
		Message  string `json:"message"`
	} `json:"issues"`
}

type exitCoder interface {
	ExitCode() int
}

func TestTicketsLintStrictReportsValidationFailures(t *testing.T) {
	repo := t.TempDir()
	configPath := writeLintConfig(t, repo)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-02": {"status": "todo", "phase": 1, "depends": [], "desc": "legacy"}
  }
}`)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--config", configPath, "tickets", "lint", "--strict", "--json=false"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected validation failure")
	}
	var coded exitCoder
	if !errors.As(err, &coded) {
		t.Fatalf("expected exit-coded error, got %T (%v)", err, err)
	}
	if coded.ExitCode() != 2 {
		t.Fatalf("exit code=%d want 2", coded.ExitCode())
	}

	output := out.String()
	mustContain := []string{"tp-02", "tickets.tp-02.objective", "tickets.tp-02.files_to_touch"}
	for _, expected := range mustContain {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestTicketsLintJSONOutputForAutomation(t *testing.T) {
	repo := t.TempDir()
	configPath := writeLintConfig(t, repo)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-02": {"status": "todo", "phase": 1, "depends": [], "desc": "legacy"}
  }
}`)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--config", configPath, "tickets", "lint", "--strict", "--json"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected validation failure")
	}

	var payload ticketLintJSON
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("expected JSON output, got err=%v output=%s", unmarshalErr, out.String())
	}
	if !payload.Strict {
		t.Fatal("strict should be true")
	}
	if payload.TicketCount != 1 {
		t.Fatalf("ticket_count=%d want 1", payload.TicketCount)
	}
	if payload.IssueCount == 0 || len(payload.Issues) == 0 {
		t.Fatalf("expected issues, payload=%+v", payload)
	}
}

func TestTicketsLintPassesForValidStrictTicket(t *testing.T) {
	repo := t.TempDir()
	configPath := writeLintConfig(t, repo)
	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "tp-02": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "type": "feature",
      "runId": "run-2026-03-04T10-32-00Z",
      "role": "backend",
      "desc": "Implement tickets lint",
      "objective": "Implement tickets lint command.",
      "scope_in": ["cmd/tickets_cmd.go"],
      "scope_out": ["No prompt generation"],
      "files_to_touch": ["cmd/tickets_cmd.go"],
      "implementation_steps": ["Write tests", "Implement command"],
      "tests_to_add_or_update": ["cmd/tickets_cmd_test.go"],
      "verify_cmd": "go test ./cmd/... ./internal/tracker/...",
      "acceptance_criteria": ["Strict lint passes"],
      "constraints": ["No prompt generation logic"]
    }
  }
}`)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--config", configPath, "tickets", "lint", "--strict", "--json=false"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "tickets lint passed") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestTicketsLintCommandIsRegistered(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"tickets", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("tickets command should be available, got error: %v\noutput:\n%s", err, out.String())
	}
}

func writeLintConfig(t *testing.T, repo string) string {
	t.Helper()
	configPath := filepath.Join(repo, "swarm.toml")
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	config := `[project]
name = "agent-swarm"
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
	return configPath
}
