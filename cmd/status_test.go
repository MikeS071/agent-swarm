package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/fatih/color"
)

func TestPrintStatusJSONIncludesRunScopeState(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"feat-a": {
			Status:  tracker.StatusTodo,
			Phase:   1,
			Depends: []string{},
			Type:    "feature",
			RunID:   "run-1",
		},
	})
	tr.CurrentRunID = "run-1"
	tr.Runs = map[string]tracker.RunState{
		"run-1": {
			Integration: tracker.StatusDone,
			PostBuild: map[string]string{
				"review": tracker.StatusTodo,
			},
		},
	}

	var out bytes.Buffer
	prev := color.Output
	color.Output = &out
	defer func() { color.Output = prev }()

	if err := printStatusJSON(tr); err != nil {
		t.Fatalf("printStatusJSON: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status json: %v", err)
	}
	if payload["currentRunId"] != "run-1" {
		t.Fatalf("currentRunId=%v want run-1", payload["currentRunId"])
	}
}
