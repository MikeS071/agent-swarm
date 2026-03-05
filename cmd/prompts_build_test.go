package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	intprompts "github.com/MikeS071/agent-swarm/internal/prompts"
)

func TestBuildPromptsAllWritesDeterministicArtifacts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "tp-03": {
      "status": "todo",
      "phase": 1,
      "depends": ["tp-01"],
      "type": "feature",
      "desc": "Implement prompts build deterministic compiler",
      "runId": "run-2026-03-04T10-32-00Z",
      "role": "backend",
      "objective": "Compile deterministic execution prompts from structured tracker fields.",
      "scope_in": ["cmd/prompts.go", "cmd/prompts_cmd.go"],
      "scope_out": ["No watchdog runtime behavior changes"],
      "files_to_touch": ["cmd/prompts.go", "cmd/prompts_cmd.go", "internal/prompts/compiler.go"],
      "reference_files": ["docs/SWARM-TICKET-PREP-V2-SPEC.md"],
      "implementation_steps": ["Add prompts build command", "Compile deterministic section order"],
      "tests_to_add_or_update": ["cmd/prompts_build_test.go", "internal/prompts/compiler_test.go"],
      "verify_cmd": "go test ./cmd/... ./internal/...",
      "acceptance_criteria": ["prompts build writes prompt and manifest files"],
      "constraints": ["Fail closed in strict mode for missing required fields"]
    },
    "tp-04": {
      "status": "todo",
      "phase": 1,
      "depends": ["tp-03"],
      "type": "feature",
      "desc": "Implement prompts validate strict mode",
      "runId": "run-2026-03-04T10-32-00Z",
      "role": "backend",
      "objective": "Implement strict validation of prompt structure.",
      "scope_in": ["cmd/prompts_cmd.go"],
      "scope_out": ["No tracker schema changes"],
      "files_to_touch": ["cmd/prompts_cmd.go"],
      "reference_files": ["docs/SWARM-TICKET-PREP-V2-SPEC.md"],
      "implementation_steps": ["Add validate command", "Add strict checks"],
      "tests_to_add_or_update": ["cmd/prompts_build_test.go"],
      "verify_cmd": "go test ./cmd/... ./internal/...",
      "acceptance_criteria": ["prompt validation catches required section issues"],
      "constraints": ["Strict mode blocks missing fields"]
    }
  }
}`)

	policy := intprompts.PolicyContext{
		ProjectName:          "agent-swarm",
		BaseBranch:           "main",
		DefaultVerifyCommand: "go test ./...",
		AgentContextPointers: []string{"swarm.toml", ".agents/AGENTS.md"},
	}

	artifacts, err := BuildPrompts(trackerPath, promptDir, "", true, true, policy)
	if err != nil {
		t.Fatalf("build prompts first run: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifact count = %d, want 2", len(artifacts))
	}

	firstPrompt, err := os.ReadFile(filepath.Join(promptDir, "tp-03.md"))
	if err != nil {
		t.Fatalf("read first prompt: %v", err)
	}
	firstManifest, err := os.ReadFile(filepath.Join(promptDir, "tp-03.manifest.json"))
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}

	if _, err := BuildPrompts(trackerPath, promptDir, "", true, true, policy); err != nil {
		t.Fatalf("build prompts second run: %v", err)
	}
	secondPrompt, err := os.ReadFile(filepath.Join(promptDir, "tp-03.md"))
	if err != nil {
		t.Fatalf("read second prompt: %v", err)
	}
	secondManifest, err := os.ReadFile(filepath.Join(promptDir, "tp-03.manifest.json"))
	if err != nil {
		t.Fatalf("read second manifest: %v", err)
	}

	if !bytes.Equal(firstPrompt, secondPrompt) {
		t.Fatal("prompt bytes changed across identical builds")
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("manifest bytes changed across identical builds")
	}
}

func TestBuildPromptsStrictFailsOnMissingRequiredData(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "tp-03": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "desc": "Incomplete ticket"
    }
  }
}`)

	_, err := BuildPrompts(trackerPath, promptDir, "tp-03", false, true, intprompts.PolicyContext{
		ProjectName: "agent-swarm",
	})
	if err == nil {
		t.Fatal("expected strict build to fail for missing required fields")
	}

	if _, statErr := os.Stat(filepath.Join(promptDir, "tp-03.md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no prompt file on strict failure, stat err=%v", statErr)
	}
}

func TestBuildPromptsStrictAllFailsClosedWithoutPartialWrites(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "tp-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "type": "feature",
      "desc": "Valid ticket",
      "runId": "run-2026-03-04T10-32-00Z",
      "role": "backend",
      "objective": "Produce deterministic output.",
      "scope_in": ["cmd/prompts.go"],
      "scope_out": ["no-op"],
      "files_to_touch": ["cmd/prompts.go"],
      "reference_files": ["SPEC.md"],
      "implementation_steps": ["step 1", "step 2"],
      "tests_to_add_or_update": ["cmd/prompts_build_test.go"],
      "verify_cmd": "go test ./cmd/... ./internal/...",
      "acceptance_criteria": ["artifact emitted"],
      "constraints": ["strict mode enabled"]
    },
    "tp-02": {
      "status": "todo",
      "phase": 1,
      "depends": ["tp-01"],
      "desc": "Invalid ticket missing required fields"
    }
  }
}`)

	_, err := BuildPrompts(trackerPath, promptDir, "", true, true, intprompts.PolicyContext{
		ProjectName: "agent-swarm",
	})
	if err == nil {
		t.Fatal("expected strict --all build to fail")
	}

	if _, statErr := os.Stat(filepath.Join(promptDir, "tp-01.md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no prompt file for valid ticket on strict --all failure, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(promptDir, "tp-01.manifest.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no manifest file for valid ticket on strict --all failure, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(promptDir, "tp-02.md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no prompt file for invalid ticket on strict --all failure, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(promptDir, "tp-02.manifest.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no manifest file for invalid ticket on strict --all failure, stat err=%v", statErr)
	}
}

func TestPromptsBuildCommandUsageAndArgs(t *testing.T) {
	t.Run("usage documents ticket or --all", func(t *testing.T) {
		if promptsBuildCmd.Use != "build <ticket|--all>" {
			t.Fatalf("prompts build usage = %q, want %q", promptsBuildCmd.Use, "build <ticket|--all>")
		}
	})

	t.Run("fails when ticket missing and --all not set", func(t *testing.T) {
		_, cfgPath := setupFeatureTestProject(t)
		_, err := runRootWithConfig(t, cfgPath, "prompts", "build")
		if err == nil {
			t.Fatal("expected error when ticket is missing")
		}
		if !strings.Contains(err.Error(), "requires exactly one ticket or --all") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("fails when ticket provided with --all", func(t *testing.T) {
		_, cfgPath := setupFeatureTestProject(t)
		_, err := runRootWithConfig(t, cfgPath, "prompts", "build", "--all", "tp-03")
		if err == nil {
			t.Fatal("expected error when ticket is provided with --all")
		}
		if !strings.Contains(err.Error(), "ticket argument is not allowed with --all") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
