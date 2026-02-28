package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptsCheckReportsMissingTodoPrompts(t *testing.T) {
	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}

	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {"status": "todo", "phase": 1, "depends": [], "branch": "feat/sw-01", "desc": "one"},
    "sw-02": {"status": "done", "phase": 1, "depends": [], "branch": "feat/sw-02", "desc": "two"},
    "sw-03": {"status": "todo", "phase": 1, "depends": ["sw-01"], "branch": "feat/sw-03", "desc": "three"}
  }
}`)
	if err := os.WriteFile(filepath.Join(promptDir, "sw-01.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	missing, err := CheckPrompts(trackerPath, promptDir)
	if err != nil {
		t.Fatalf("check prompts: %v", err)
	}
	if len(missing) != 1 || missing[0] != "sw-03" {
		t.Fatalf("unexpected missing prompts: %#v", missing)
	}
}

func TestPromptsGenCreatesTemplate(t *testing.T) {
	repo := t.TempDir()
	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	promptDir := filepath.Join(repo, "swarm", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}

	writeJSON(t, trackerPath, `{
  "project": "agent-swarm",
  "tickets": {
    "sw-04": {"status": "todo", "phase": 1, "depends": ["sw-01", "sw-03"], "branch": "feat/sw-04", "desc": "Worktree manager"}
  }
}`)

	promptPath, err := GeneratePrompt(trackerPath, promptDir, "sw-04")
	if err != nil {
		t.Fatalf("generate prompt: %v", err)
	}
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# SW-04") {
		t.Fatalf("missing heading: %s", content)
	}
	if !strings.Contains(content, "Worktree manager") {
		t.Fatalf("missing description: %s", content)
	}
	if !strings.Contains(content, "sw-01, sw-03") {
		t.Fatalf("missing deps list: %s", content)
	}
	if !strings.Contains(content, "## Context") || !strings.Contains(content, "## Your Scope") {
		t.Fatalf("missing template sections: %s", content)
	}
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
