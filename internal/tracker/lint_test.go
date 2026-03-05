package tracker

import (
	"strings"
	"testing"
)

func TestLintTickets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		strict        bool
		contents      string
		wantIssueCnt  int
		wantPathParts []string
	}{
		{
			name:   "strict valid v2 ticket",
			strict: true,
			contents: `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"type":"feature","runId":"run-2026-03-04T10-32-00Z","role":"backend","desc":"Implement schema validator","objective":"Implement schema validator.","scope_in":["internal/guardian/schema/load.go"],"scope_out":["No CLI changes"],"files_to_touch":["internal/guardian/schema/*.go"],"implementation_steps":["Define policy structs","Implement YAML loader"],"tests_to_add_or_update":["internal/guardian/schema/validate_test.go"],"verify_cmd":"go test ./internal/guardian/schema/...","acceptance_criteria":["Valid policy loads","Invalid policy fails"],"constraints":["No runtime dependency changes"]}}}`,
			wantIssueCnt: 0,
		},
		{
			name:   "strict mode flags missing required v2 fields",
			strict: true,
			contents: `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"desc":"legacy ticket"}}}`,
			wantIssueCnt: 12,
			wantPathParts: []string{
				"tickets.t1.type",
				"tickets.t1.runId",
				"tickets.t1.role",
				"tickets.t1.objective",
				"tickets.t1.scope_in",
				"tickets.t1.scope_out",
				"tickets.t1.files_to_touch",
				"tickets.t1.implementation_steps",
				"tickets.t1.tests_to_add_or_update",
				"tickets.t1.verify_cmd",
				"tickets.t1.acceptance_criteria",
				"tickets.t1.constraints",
			},
		},
		{
			name:   "non strict allows legacy ticket",
			strict: false,
			contents: `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"desc":"legacy ticket"}}}`,
			wantIssueCnt: 0,
		},
		{
			name:   "present fields still validated in non strict mode",
			strict: false,
			contents: `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"desc":"x","objective":" ","files_to_touch":[""],"implementation_steps":["one"],"verify_cmd":" ","acceptance_criteria":[""],"constraints":[]}}}`,
			wantIssueCnt: 6,
			wantPathParts: []string{
				"tickets.t1.objective",
				"tickets.t1.files_to_touch[0]",
				"tickets.t1.implementation_steps",
				"tickets.t1.verify_cmd",
				"tickets.t1.acceptance_criteria[0]",
				"tickets.t1.constraints",
			},
		},
		{
			name:   "reports invalid tracker tickets shape",
			strict: true,
			contents: `{"project":"x","tickets":[]}`,
			wantIssueCnt: 1,
			wantPathParts: []string{
				"tickets",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeTrackerFile(t, tc.contents)

			report, err := LintTickets(path, tc.strict)
			if err != nil {
				t.Fatalf("LintTickets() error = %v", err)
			}

			if report.Strict != tc.strict {
				t.Fatalf("strict=%v want %v", report.Strict, tc.strict)
			}
			if got := len(report.Issues); got != tc.wantIssueCnt {
				t.Fatalf("issue count=%d want %d, issues=%+v", got, tc.wantIssueCnt, report.Issues)
			}

			if tc.wantIssueCnt > 0 {
				joined := report.String()
				for _, pathPart := range tc.wantPathParts {
					if !strings.Contains(joined, pathPart) {
						t.Fatalf("report missing %q: %s", pathPart, joined)
					}
				}
			}
		})
	}
}

func TestLintTicketsRejectsUnreadableFile(t *testing.T) {
	t.Parallel()

	if _, err := LintTickets("/does/not/exist/tracker.json", true); err == nil {
		t.Fatal("expected read error")
	}
}

func TestLintTicketsSortsIssuesDeterministically(t *testing.T) {
	t.Parallel()

	path := writeTrackerFile(t, `{"project":"x","tickets":{"b":{"status":"todo","phase":1,"depends":[]},"a":{"status":"todo","phase":1,"depends":[]}}}`)
	report, err := LintTickets(path, true)
	if err != nil {
		t.Fatalf("LintTickets() error = %v", err)
	}
	if len(report.Issues) < 2 {
		t.Fatalf("expected issues, got %+v", report.Issues)
	}
	if report.Issues[0].Path > report.Issues[1].Path {
		t.Fatalf("issues not sorted: %+v", report.Issues)
	}
}
