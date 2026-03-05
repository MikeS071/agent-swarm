package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTicketsLintFixture(t *testing.T, trackerJSON string) (repo string, cfgPath string) {
	t.Helper()
	repo = t.TempDir()
	cfgPath = filepath.Join(repo, "swarm.toml")
	cfg := `[project]
name = "proj"
repo = "."
base_branch = "main"
prompt_dir = "swarm/prompts"
tracker = "swarm/tracker.json"
require_explicit_role = true
require_verify_cmd = true

[lifecycle]
policy_file = ".agents/lifecycle-policy.toml"

[integration]
verify_cmd = ""
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "swarm"), 0o755); err != nil {
		t.Fatalf("mkdir swarm: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".agents", "profiles"), 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	policy := `[profiles.by_ticket_type]
int = "code-agent"
gap = "code-reviewer"
tst = "e2e-runner"
review = "code-reviewer"
sec = "security-reviewer"
doc = "doc-updater"
clean = "refactor-cleaner"
mem = "doc-updater"
`
	if err := os.WriteFile(filepath.Join(repo, ".agents", "lifecycle-policy.toml"), []byte(policy), 0o644); err != nil {
		t.Fatalf("write lifecycle policy: %v", err)
	}
	profile := `---
name: code-agent
description: test profile
mode: Development
---
`
	if err := os.WriteFile(filepath.Join(repo, ".agents", "profiles", "code-agent.md"), []byte(profile), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "swarm", "tracker.json"), []byte(trackerJSON), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	return repo, cfgPath
}

func TestTicketsLintPassesForValidStrictTicket(t *testing.T) {
	repo, cfgPath := writeTicketsLintFixture(t, `{
  "project":"proj",
  "tickets":{
    "v2-01":{"status":"todo","phase":1,"branch":"feat/v2-01","desc":"Ticket lint","profile":"code-agent","verify_cmd":"go test ./cmd/... -run TicketsLint"}
  }
}`)
	prompt := `# v2-01

## Objective
Do work

## Dependencies
none

## Scope
cmd/*

## Verify
go test ./cmd/... -run TicketsLint
`
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompts", "v2-01.md"), []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	out, err := runRootWithConfig(t, cfgPath, "tickets", "lint")
	if err != nil {
		t.Fatalf("tickets lint returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tickets lint ok") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestTicketsLintStrictReportsValidationFailures(t *testing.T) {
	_, cfgPath := writeTicketsLintFixture(t, `{
  "project":"proj",
  "tickets":{
    "v2-01":{"status":"todo","phase":0,"depends":["missing","missing","v2-01"],"branch":"","desc":"","profile":"","verify_cmd":""}
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "tickets", "lint")
	if err == nil {
		t.Fatalf("expected error, got nil with output: %s", out)
	}
	if !strings.Contains(out, "missing explicit role/profile") {
		t.Fatalf("expected profile failure in output:\n%s", out)
	}
	if !strings.Contains(out, "unknown dependency \"missing\"") {
		t.Fatalf("expected dependency failure in output:\n%s", out)
	}
	if !strings.Contains(out, "phase must be > 0") {
		t.Fatalf("expected phase failure in output:\n%s", out)
	}
}

func TestTicketsLintJSONOutputForAutomation(t *testing.T) {
	_, cfgPath := writeTicketsLintFixture(t, `{
  "project":"proj",
  "tickets":{
    "v2-01":{"status":"todo","phase":0,"branch":"","desc":"","profile":"","verify_cmd":""}
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "tickets", "lint", "--json")
	if err == nil {
		t.Fatalf("expected lint error for invalid ticket")
	}
	var payload struct {
		OK     bool               `json:"ok"`
		Issues []ticketsLintIssue `json:"issues"`
	}
	if jerr := json.Unmarshal([]byte(out), &payload); jerr != nil {
		t.Fatalf("failed to parse json output: %v\n%s", jerr, out)
	}
	if payload.OK {
		t.Fatalf("expected ok=false, got true")
	}
	if len(payload.Issues) == 0 {
		t.Fatalf("expected non-empty issues")
	}
}

func TestTicketsLintCommandIsRegistered(t *testing.T) {
	var foundTickets bool
	var foundLint bool
	for _, c := range rootCmd.Commands() {
		if c.Name() != "tickets" {
			continue
		}
		foundTickets = true
		for _, sc := range c.Commands() {
			if sc.Name() == "lint" {
				foundLint = true
			}
		}
	}
	if !foundTickets || !foundLint {
		t.Fatalf("tickets/lint command registration missing (tickets=%v lint=%v)", foundTickets, foundLint)
	}
}
