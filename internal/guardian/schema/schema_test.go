package schema

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		wantErr    bool
		wantIssues []string
	}{
		{
			name: "happy path valid schema",
			content: `
version: 1
name: default-v2-flow
modes:
  default: advisory
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions:
  - from: draft
    to: planned
    requires:
      rules:
        - type: ticket_desc_has_scope_and_verify
`,
		},
		{
			name: "error missing required keys",
			content: `
name: invalid-flow
modes:
  default: advisory
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions: []
`,
			wantErr:    true,
			wantIssues: []string{"version", "transitions"},
		},
		{
			name: "edge unknown rule and bad transition",
			content: `
version: 1
name: bad-flow
modes:
  default: advisory
state:
  initial: draft
  terminal: [complete]
phases:
  - id: planning
    states: [draft, planned]
transitions:
  - from: unknown
    to: planned
    requires:
      rules:
        - type: does_not_exist
`,
			wantErr:    true,
			wantIssues: []string{"unknown rule", "transition.from"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "flow.v2.yaml")
			writeFlow(t, path, tt.content)

			flow, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() expected error")
				}
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("Load() error type = %T, want *ValidationError", err)
				}
				full := err.Error()
				for _, issue := range tt.wantIssues {
					if !strings.Contains(full, issue) {
						t.Fatalf("Load() error %q does not contain %q", full, issue)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if flow == nil {
				t.Fatalf("Load() flow is nil")
			}
			if flow.Version != 1 {
				t.Fatalf("flow.Version = %d, want 1", flow.Version)
			}
			if flow.Modes.Default != "advisory" {
				t.Fatalf("flow.Modes.Default = %q, want advisory", flow.Modes.Default)
			}
		})
	}
}

func writeFlow(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}
}
