package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/guardian/engine"
)

func TestEventWriterAppendDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		decision engine.Decision
		wantFile bool
	}{
		{
			name: "happy path appends jsonl",
			path: filepath.Join(t.TempDir(), "guardian-events.jsonl"),
			decision: engine.Decision{
				Result:   engine.ResultWarn,
				Rule:     "ticket_desc_has_scope_and_verify",
				Reason:   "missing verify section",
				Target:   "ticket:sw-01",
				Evidence: "state/evidence/sw-01.json",
				Time:     time.Unix(1700000000, 0).UTC(),
			},
			wantFile: true,
		},
		{
			name: "edge zero timestamp auto-filled",
			path: filepath.Join(t.TempDir(), "guardian-events.jsonl"),
			decision: engine.Decision{
				Result: engine.ResultAllow,
				Rule:   "flow_schema_valid",
				Target: "project:test",
			},
			wantFile: true,
		},
		{
			name: "error path empty path is no-op",
			path: "",
			decision: engine.Decision{
				Result: engine.ResultWarn,
				Rule:   "flow_schema_valid",
			},
			wantFile: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewEventWriter(tt.path)
			if err := w.AppendDecision(tt.decision); err != nil {
				t.Fatalf("AppendDecision() error = %v", err)
			}

			if !tt.wantFile {
				return
			}

			b, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read events file: %v", err)
			}
			line := strings.TrimSpace(string(b))
			if line == "" {
				t.Fatalf("expected one jsonl line")
			}

			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("unmarshal event line: %v", err)
			}
			if ev.Rule != tt.decision.Rule {
				t.Fatalf("event rule = %q, want %q", ev.Rule, tt.decision.Rule)
			}
			if ev.Result != tt.decision.Result {
				t.Fatalf("event result = %q, want %q", ev.Result, tt.decision.Result)
			}
			if ev.Time.IsZero() {
				t.Fatalf("event time must be set")
			}
		})
	}
}
