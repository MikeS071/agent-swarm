package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewPostBuildRunScopeDetectsLegacyWithoutMutation(t *testing.T) {
	trackerPath := writeTrackerJSON(t, `{
  "project": "proj",
  "tickets": {
    "feat-a-01": {"status": "done", "phase": 1, "depends": [], "type": "build", "feature": "a"},
    "feat-b-01": {"status": "done", "phase": 1, "depends": [], "type": "build", "feature": "b"},
    "int-a": {"status": "done", "phase": 2, "depends": ["feat-a-01"], "type": "int", "feature": "a"},
    "gap-a": {"status": "todo", "phase": 2, "depends": ["int-a"], "type": "gap", "feature": "a"},
    "tst-a": {"status": "done", "phase": 2, "depends": ["int-a"], "type": "tst", "feature": "a"},
    "int-b": {"status": "done", "phase": 2, "depends": ["feat-b-01"], "type": "int", "feature": "b"},
    "gap-b": {"status": "done", "phase": 2, "depends": ["int-b"], "type": "gap", "feature": "b"},
    "tst-b": {"status": "running", "phase": 2, "depends": ["int-b"], "type": "tst", "feature": "b"},
    "deploy": {"status": "todo", "phase": 3, "depends": ["gap-a", "tst-a", "gap-b", "tst-b"], "type": "feature"}
  }
}`)
	before := readFile(t, trackerPath)

	res, err := PreviewPostBuildRunScope(trackerPath, PostBuildRunScopeOptions{
		Steps: []string{"int", "gap", "tst"},
		RunID: "run-legacy-42",
	})
	if err != nil {
		t.Fatalf("preview migration: %v", err)
	}

	if !res.LegacyDetected {
		t.Fatalf("LegacyDetected = false, want true")
	}
	if res.LegacyTicketCount != 6 {
		t.Fatalf("LegacyTicketCount = %d, want 6", res.LegacyTicketCount)
	}
	if res.RemovedDependencyCount != 4 {
		t.Fatalf("RemovedDependencyCount = %d, want 4", res.RemovedDependencyCount)
	}
	if res.IntegrationStatus != "done" {
		t.Fatalf("IntegrationStatus = %q, want done", res.IntegrationStatus)
	}
	if res.PostBuildStatuses["gap"] != "running" {
		t.Fatalf("gap status = %q, want running", res.PostBuildStatuses["gap"])
	}
	if res.PostBuildStatuses["tst"] != "running" {
		t.Fatalf("tst status = %q, want running", res.PostBuildStatuses["tst"])
	}

	after := readFile(t, trackerPath)
	if before != after {
		t.Fatalf("preview mutated tracker file")
	}
}

func TestApplyPostBuildRunScopeWritesTrackerTransformation(t *testing.T) {
	trackerPath := writeTrackerJSON(t, `{
  "project": "proj",
  "tickets": {
    "feat-a-01": {"status": "done", "phase": 1, "depends": [], "type": "build", "feature": "a"},
    "int-a": {"status": "done", "phase": 2, "depends": ["feat-a-01"], "type": "int", "feature": "a"},
    "gap-a": {"status": "done", "phase": 2, "depends": ["int-a"], "type": "gap", "feature": "a"},
    "tst-a": {"status": "done", "phase": 2, "depends": ["int-a"], "type": "tst", "feature": "a"},
    "deploy": {"status": "todo", "phase": 3, "depends": ["gap-a", "tst-a"], "type": "feature"}
  }
}`)

	res, err := ApplyPostBuildRunScope(trackerPath, PostBuildRunScopeOptions{Steps: []string{"int", "gap", "tst"}, RunID: "run-legacy-1"})
	if err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	if !res.LegacyDetected {
		t.Fatalf("LegacyDetected = false, want true")
	}

	var doc struct {
		CurrentRunID string `json:"currentRunId"`
		Runs         map[string]struct {
			Integration struct {
				Status string `json:"status"`
			} `json:"integration"`
			PostBuild map[string]struct {
				Status string `json:"status"`
			} `json:"postBuild"`
		} `json:"runs"`
		Tickets map[string]struct {
			Depends []string `json:"depends"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(readFile(t, trackerPath)), &doc); err != nil {
		t.Fatalf("unmarshal transformed tracker: %v", err)
	}

	if doc.CurrentRunID != "run-legacy-1" {
		t.Fatalf("currentRunId = %q, want run-legacy-1", doc.CurrentRunID)
	}
	run, ok := doc.Runs["run-legacy-1"]
	if !ok {
		t.Fatalf("runs missing run-legacy-1")
	}
	if run.Integration.Status != "done" {
		t.Fatalf("integration.status = %q, want done", run.Integration.Status)
	}
	if run.PostBuild["gap"].Status != "done" {
		t.Fatalf("postBuild.gap.status = %q, want done", run.PostBuild["gap"].Status)
	}
	if run.PostBuild["tst"].Status != "done" {
		t.Fatalf("postBuild.tst.status = %q, want done", run.PostBuild["tst"].Status)
	}
	if _, ok := doc.Tickets["int-a"]; ok {
		t.Fatalf("legacy ticket int-a should be removed")
	}
	if _, ok := doc.Tickets["gap-a"]; ok {
		t.Fatalf("legacy ticket gap-a should be removed")
	}
	if _, ok := doc.Tickets["tst-a"]; ok {
		t.Fatalf("legacy ticket tst-a should be removed")
	}
	if got := doc.Tickets["deploy"].Depends; len(got) != 0 {
		t.Fatalf("deploy.depends = %v, want []", got)
	}
}

func TestPreviewPostBuildRunScopeNoLegacyGraphNoChanges(t *testing.T) {
	trackerPath := writeTrackerJSON(t, `{
  "project": "proj",
  "tickets": {
    "feat-a-01": {"status": "todo", "phase": 1, "depends": [], "type": "feature"},
    "feat-a-02": {"status": "todo", "phase": 1, "depends": ["feat-a-01"], "type": "feature"}
  }
}`)

	res, err := PreviewPostBuildRunScope(trackerPath, PostBuildRunScopeOptions{Steps: []string{"int", "gap", "tst"}})
	if err != nil {
		t.Fatalf("preview migration: %v", err)
	}
	if res.LegacyDetected {
		t.Fatalf("LegacyDetected = true, want false")
	}
	if res.LegacyTicketCount != 0 {
		t.Fatalf("LegacyTicketCount = %d, want 0", res.LegacyTicketCount)
	}
	if !strings.Contains(FormatPostBuildRunScopeSummary(res, false), "No legacy") {
		t.Fatalf("expected no-legacy summary")
	}
}

func TestPreviewPostBuildRunScopeInvalidTrackerJSON(t *testing.T) {
	trackerPath := filepath.Join(t.TempDir(), "tracker.json")
	if err := os.WriteFile(trackerPath, []byte(`{"project"`), 0o644); err != nil {
		t.Fatalf("write invalid tracker: %v", err)
	}

	_, err := PreviewPostBuildRunScope(trackerPath, PostBuildRunScopeOptions{Steps: []string{"int", "gap", "tst"}})
	if err == nil {
		t.Fatalf("expected error for invalid tracker JSON")
	}
}

func writeTrackerJSON(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tracker.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
