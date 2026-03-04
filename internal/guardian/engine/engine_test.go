package engine

import "testing"

func TestEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       Mode
		checks     []Check
		wantResult Result
	}{
		{
			name: "happy path all checks pass",
			mode: ModeAdvisory,
			checks: []Check{
				{Rule: "ticket_desc_has_scope_and_verify", Passed: true, Target: "ticket:sw-01"},
			},
			wantResult: ResultAllow,
		},
		{
			name: "error path failed check blocks in enforce mode",
			mode: ModeEnforce,
			checks: []Check{
				{Rule: "ticket_desc_has_scope_and_verify", Passed: false, Reason: "missing verify", Target: "ticket:sw-01"},
			},
			wantResult: ResultBlock,
		},
		{
			name: "edge failed check only warns in advisory mode",
			mode: ModeAdvisory,
			checks: []Check{
				{Rule: "ticket_desc_has_scope_and_verify", Passed: false, Reason: "missing scope", Target: "ticket:sw-01"},
			},
			wantResult: ResultWarn,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decisions := Evaluate(tt.mode, tt.checks)
			if len(decisions) != len(tt.checks) {
				t.Fatalf("len(decisions) = %d, want %d", len(decisions), len(tt.checks))
			}
			if got := Overall(decisions); got != tt.wantResult {
				t.Fatalf("Overall() = %q, want %q", got, tt.wantResult)
			}
		})
	}
}

func TestOverall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decisions []Decision
		want      Result
	}{
		{
			name: "edge empty decisions defaults allow",
			want: ResultAllow,
		},
		{
			name: "warn dominates allow",
			decisions: []Decision{
				{Result: ResultAllow},
				{Result: ResultWarn},
			},
			want: ResultWarn,
		},
		{
			name: "block dominates all",
			decisions: []Decision{
				{Result: ResultAllow},
				{Result: ResultWarn},
				{Result: ResultBlock},
			},
			want: ResultBlock,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Overall(tt.decisions); got != tt.want {
				t.Fatalf("Overall() = %q, want %q", got, tt.want)
			}
		})
	}
}
