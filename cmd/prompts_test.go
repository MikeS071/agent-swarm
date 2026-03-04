package cmd

import (
	"os"
	"path/filepath"
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

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
