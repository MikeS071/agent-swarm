package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchBlocksByDefaultWhenPrepFails(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{backendType: "invalid-backend"})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/sw-01",
      "desc": "ticket"
    }
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "watch", "--once", "--dry-run")
	if err == nil {
		t.Fatalf("expected watch to fail prep gate")
	}
	if !strings.Contains(err.Error(), "prep gate failed") {
		t.Fatalf("expected prep gate failure, got: %v", err)
	}
	if strings.Contains(err.Error(), "unsupported backend type") {
		t.Fatalf("watch should fail at prep gate before backend build, got: %v", err)
	}
	if !strings.Contains(out, "prep") {
		t.Fatalf("expected prep guidance in output, got:\n%s", out)
	}
}

func TestWatchAllowUnpreparedOverrideSkipsPrepGate(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{backendType: "invalid-backend"})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/sw-01",
      "desc": "ticket"
    }
  }
}`)

	out, err := runRootWithConfig(t, cfgPath, "watch", "--once", "--dry-run", "--allow-unprepared")
	if err == nil {
		t.Fatalf("expected watch error from unsupported backend type")
	}
	if !strings.Contains(err.Error(), "unsupported backend type") {
		t.Fatalf("expected backend error after prep override, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "warning") {
		t.Fatalf("expected loud warning in output when override is used, got:\n%s", out)
	}
	if !strings.Contains(out, "allow-unprepared") {
		t.Fatalf("expected output to mention allow-unprepared override, got:\n%s", out)
	}
}

func TestWatchProceedsWhenPrepPasses(t *testing.T) {
	repo, cfgPath := setupPrepProject(t, prepProjectOptions{backendType: "invalid-backend"})

	writeJSON(t, filepath.Join(repo, "swarm", "tracker.json"), `{
  "project": "agent-swarm",
  "tickets": {
    "sw-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/sw-01",
      "desc": "ticket",
      "profile": "code-agent",
      "verify_cmd": "go test ./..."
    }
  }
}`)
	writePath(t, filepath.Join(repo, "swarm", "prompts", "sw-01.md"), "# sw-01\n")

	out, err := runRootWithConfig(t, cfgPath, "watch", "--once", "--dry-run")
	if err == nil {
		t.Fatalf("expected watch to fail on invalid backend type")
	}
	if strings.Contains(err.Error(), "prep gate failed") {
		t.Fatalf("prep should pass in this scenario, got error: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "unsupported backend type") {
		t.Fatalf("expected backend build failure, got: %v", err)
	}
}
