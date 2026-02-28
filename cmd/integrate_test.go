package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrateDryRunOrdersDoneTicketsByDependency(t *testing.T) {
	repo := initRepo(t)
	addLocalOrigin(t, repo)

	createFeatureBranch(t, repo, "feat/sw-01", map[string]string{"one.txt": "1\n"})
	createFeatureBranch(t, repo, "feat/sw-02", map[string]string{"two.txt": "2\n"})
	createFeatureBranch(t, repo, "feat/sw-03", map[string]string{"three.txt": "3\n"})

	writeSwarmConfig(t, repo, "")
	writeTrackerJSON(t, repo, `{
  "project": "test",
  "tickets": {
    "sw-03": {"status": "done", "phase": 1, "depends": ["sw-02"], "branch": "feat/sw-03"},
    "sw-01": {"status": "done", "phase": 1, "depends": [], "branch": "feat/sw-01"},
    "sw-02": {"status": "done", "phase": 1, "depends": ["sw-01"], "branch": "feat/sw-02"},
    "sw-04": {"status": "todo", "phase": 1, "depends": ["sw-03"], "branch": "feat/sw-04"}
  }
}`)

	var out bytes.Buffer
	runner := newIntegrateRunner(repo, "main", "integration/v1", true, false, &out)
	if err := runner.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	body := out.String()
	i1 := strings.Index(body, "sw-01")
	i2 := strings.Index(body, "sw-02")
	i3 := strings.Index(body, "sw-03")
	if !(i1 >= 0 && i2 > i1 && i3 > i2) {
		t.Fatalf("unexpected merge order output:\n%s", body)
	}
	if strings.Contains(body, "sw-04") {
		t.Fatalf("todo ticket should not be included in merge summary: %s", body)
	}
}

func TestIntegrateConflictSavesState(t *testing.T) {
	repo := initRepo(t)
	addLocalOrigin(t, repo)

	writePath(t, filepath.Join(repo, "shared.txt"), "base\n")
	runCmd(t, repo, "git", "add", "shared.txt")
	runCmd(t, repo, "git", "commit", "-m", "add shared")

	createFeatureBranch(t, repo, "feat/sw-01", map[string]string{"shared.txt": "from sw-01\n"})
	createFeatureBranch(t, repo, "feat/sw-02", map[string]string{"shared.txt": "from sw-02\n"})

	writeSwarmConfig(t, repo, "")
	writeTrackerJSON(t, repo, `{
  "project": "test",
  "tickets": {
    "sw-01": {"status": "done", "phase": 1, "depends": [], "branch": "feat/sw-01"},
    "sw-02": {"status": "done", "phase": 1, "depends": ["sw-01"], "branch": "feat/sw-02"}
  }
}`)

	var out bytes.Buffer
	runner := newIntegrateRunner(repo, "main", "integration/v1", false, false, &out)
	err := runner.Run()
	if err == nil {
		t.Fatalf("expected conflict error")
	}

	statePath := filepath.Join(repo, integrateStateFilename)
	stateData, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("expected state file at conflict: %v", readErr)
	}
	var state integrateState
	if jsonErr := json.Unmarshal(stateData, &state); jsonErr != nil {
		t.Fatalf("parse state: %v", jsonErr)
	}
	if state.NextIndex != 2 {
		t.Fatalf("NextIndex = %d, want 2", state.NextIndex)
	}
	if len(state.Tickets) != 2 || state.Tickets[1] != "sw-02" {
		t.Fatalf("unexpected state tickets: %#v", state.Tickets)
	}
	if !strings.Contains(out.String(), "Conflicted files") || !strings.Contains(out.String(), "shared.txt") {
		t.Fatalf("conflict output missing file details:\n%s", out.String())
	}
}

func TestIntegrateContinueResumesFromState(t *testing.T) {
	repo := initRepo(t)
	addLocalOrigin(t, repo)

	writePath(t, filepath.Join(repo, "shared.txt"), "base\n")
	runCmd(t, repo, "git", "add", "shared.txt")
	runCmd(t, repo, "git", "commit", "-m", "add shared")

	createFeatureBranch(t, repo, "feat/sw-01", map[string]string{"shared.txt": "from sw-01\n"})
	createFeatureBranch(t, repo, "feat/sw-02", map[string]string{"shared.txt": "from sw-02\n"})
	createFeatureBranch(t, repo, "feat/sw-03", map[string]string{"extra.txt": "ok\n"})

	writeSwarmConfig(t, repo, "")
	writeTrackerJSON(t, repo, `{
  "project": "test",
  "tickets": {
    "sw-01": {"status": "done", "phase": 1, "depends": [], "branch": "feat/sw-01"},
    "sw-02": {"status": "done", "phase": 1, "depends": ["sw-01"], "branch": "feat/sw-02"},
    "sw-03": {"status": "done", "phase": 1, "depends": ["sw-02"], "branch": "feat/sw-03"}
  }
}`)

	first := newIntegrateRunner(repo, "main", "integration/v1", false, false, bytes.NewBuffer(nil))
	if err := first.Run(); err == nil {
		t.Fatalf("expected initial run to stop at conflict")
	}

	writePath(t, filepath.Join(repo, "shared.txt"), "resolved\n")
	runCmd(t, repo, "git", "add", "shared.txt")
	runCmd(t, repo, "git", "commit", "-m", "resolve sw-02 conflict")

	var out bytes.Buffer
	cont := newIntegrateRunner(repo, "", "", false, true, &out)
	if err := cont.Run(); err != nil {
		t.Fatalf("continue Run() error = %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(repo, integrateStateFilename)); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be removed after successful continue")
	}

	if !strings.Contains(runCmd(t, repo, "git", "branch", "--contains", "feat/sw-03"), "integration/v1") {
		t.Fatalf("integration branch does not include sw-03 merge")
	}
}

func addLocalOrigin(t *testing.T, repo string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "origin.git")
	runCmd(t, filepath.Dir(remote), "git", "init", "--bare", remote)
	runCmd(t, repo, "git", "remote", "add", "origin", remote)
	runCmd(t, repo, "git", "push", "-u", "origin", "main")
}

func createFeatureBranch(t *testing.T, repo, branch string, files map[string]string) {
	t.Helper()
	runCmd(t, repo, "git", "checkout", "-B", branch, "main")
	for rel, body := range files {
		writePath(t, filepath.Join(repo, rel), body)
		runCmd(t, repo, "git", "add", rel)
	}
	runCmd(t, repo, "git", "commit", "-m", "commit "+branch)
	runCmd(t, repo, "git", "checkout", "main")
}

func writeSwarmConfig(t *testing.T, repo, verifyCmd string) {
	t.Helper()
	cfg := `[project]
name = "test"
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

[integration]
verify_cmd = "` + verifyCmd + `"
audit_ticket = "sw-audit"
`
	writePath(t, filepath.Join(repo, "swarm.toml"), cfg)
}

func writeTrackerJSON(t *testing.T, repo, content string) {
	t.Helper()
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), content)
}

func writePath(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
