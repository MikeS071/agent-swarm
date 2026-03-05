package tui

import (
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestRenderListShowsSeparateFeatureAndRunProgress(t *testing.T) {
	cfg := config.Default()
	cfg.Project.Name = "demo"
	cfg.PostBuild.Order = []string{"review", "sec", "doc"}

	tr := tracker.New("demo", map[string]tracker.Ticket{
		"cache-1":      {Status: tracker.StatusDone, Phase: 1, Type: "feature", Feature: "cache", RunID: "run-1", Desc: "cache build"},
		"billing-1":    {Status: tracker.StatusRunning, Phase: 1, Type: "feature", Feature: "billing", RunID: "run-1", Desc: "billing build"},
		"int-run-1":    {Status: tracker.StatusDone, Phase: 2, Type: "integration", RunID: "run-1", Desc: "integration"},
		"review-run-1": {Status: tracker.StatusDone, Phase: 3, Type: "review", RunID: "run-1", Desc: "review"},
		"sec-run-1":    {Status: tracker.StatusRunning, Phase: 3, Type: "sec", RunID: "run-1", Desc: "security"},
	})

	m := model{
		config:   cfg,
		tracker:  tr,
		pageSize: 20,
		width:    140,
	}
	m.rebuildRows()

	out := m.renderList()
	if !strings.Contains(out, "Feature Progress:") {
		t.Fatalf("expected feature progress block in output:\n%s", out)
	}
	if !strings.Contains(out, "Run Progress:") {
		t.Fatalf("expected run progress block in output:\n%s", out)
	}
	if !strings.Contains(out, "review=done") {
		t.Fatalf("expected review post-build step status in output:\n%s", out)
	}
	if !strings.Contains(out, "sec=running") {
		t.Fatalf("expected sec post-build step status in output:\n%s", out)
	}
	if !strings.Contains(out, "doc=skipped") {
		t.Fatalf("expected missing post-build step to render as skipped in output:\n%s", out)
	}
}
