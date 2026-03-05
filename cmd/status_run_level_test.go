package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/fatih/color"
)

func runLevelTrackerFixture() *tracker.Tracker {
	return tracker.New("demo", map[string]tracker.Ticket{
		"cache-1":      {Status: tracker.StatusDone, Phase: 1, Type: "feature", Feature: "cache", RunID: "run-1", Desc: "cache build"},
		"billing-1":    {Status: tracker.StatusRunning, Phase: 1, Type: "feature", Feature: "billing", RunID: "run-1", Desc: "billing build"},
		"int-run-1":    {Status: tracker.StatusDone, Phase: 2, Type: "integration", RunID: "run-1", Desc: "integration"},
		"review-run-1": {Status: tracker.StatusDone, Phase: 3, Type: "review", RunID: "run-1", Desc: "review"},
		"sec-run-1":    {Status: tracker.StatusRunning, Phase: 3, Type: "sec", RunID: "run-1", Desc: "security"},
	})
}

func TestPrintStatusJSONIncludesRunLevelPostBuildProgress(t *testing.T) {
	tr := runLevelTrackerFixture()
	cfg := config.Default()
	cfg.PostBuild.Order = []string{"review", "sec", "doc"}

	var out bytes.Buffer
	prev := color.Output
	color.Output = &out
	defer func() { color.Output = prev }()

	if err := printStatusJSON(cfg, tr); err != nil {
		t.Fatalf("printStatusJSON() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}

	if _, ok := payload["featureStats"]; !ok {
		t.Fatalf("expected featureStats key, got %v", payload)
	}
	rawRun, ok := payload["runProgress"]
	if !ok {
		t.Fatalf("expected runProgress key, got %v", payload)
	}
	runProgress, ok := rawRun.(map[string]any)
	if !ok {
		t.Fatalf("runProgress type = %T, want object", rawRun)
	}
	if got := runProgress["integration_status"]; got != "done" {
		t.Fatalf("integration_status = %v, want done", got)
	}
	rawSteps, ok := runProgress["post_build_steps"]
	if !ok {
		t.Fatalf("expected post_build_steps in runProgress, got %v", runProgress)
	}
	steps, ok := rawSteps.(map[string]any)
	if !ok {
		t.Fatalf("post_build_steps type = %T, want object", rawSteps)
	}
	if got := steps["review"]; got != "done" {
		t.Fatalf("review status = %v, want done", got)
	}
	if got := steps["sec"]; got != "running" {
		t.Fatalf("sec status = %v, want running", got)
	}
	if got := steps["doc"]; got != "skipped" {
		t.Fatalf("doc status = %v, want skipped", got)
	}
}

func TestPrintStatusTableShowsFeatureVsRunProgress(t *testing.T) {
	tr := runLevelTrackerFixture()
	cfg := config.Default()
	cfg.PostBuild.Order = []string{"review", "sec", "doc"}

	var out bytes.Buffer
	prev := color.Output
	color.Output = &out
	defer func() { color.Output = prev }()

	printStatusTable(cfg, tr)
	body := out.String()

	if !strings.Contains(body, "Feature stats:") {
		t.Fatalf("expected feature stats block in output:\n%s", body)
	}
	if !strings.Contains(body, "Run progress:") {
		t.Fatalf("expected run progress block in output:\n%s", body)
	}
	if !strings.Contains(body, "review=done") {
		t.Fatalf("expected review post-build status in output:\n%s", body)
	}
	if !strings.Contains(body, "sec=running") {
		t.Fatalf("expected sec post-build status in output:\n%s", body)
	}
	if !strings.Contains(body, "doc=skipped") {
		t.Fatalf("expected skipped post-build status in output:\n%s", body)
	}
}
