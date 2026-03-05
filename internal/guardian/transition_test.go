package guardian

import (
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestCollectTransitionUnmetConditions(t *testing.T) {
	tests := []struct {
		name  string
		input TransitionCheckInput
		want  []string
	}{
		{
			name: "returns nil when no tickets",
			input: TransitionCheckInput{
				Tickets:             map[string]tracker.Ticket{},
				RequireExplicitRole: true,
				RequireVerifyCmd:    true,
			},
			want: nil,
		},
		{
			name: "reports both unmet conditions for todo ticket",
			input: TransitionCheckInput{
				Tickets: map[string]tracker.Ticket{
					"sw-02": {Status: tracker.StatusTodo, Phase: 2},
				},
				RequireExplicitRole: true,
				RequireVerifyCmd:    true,
			},
			want: []string{
				"ticket sw-02 missing explicit role/profile",
				"ticket sw-02 missing verify_cmd",
			},
		},
		{
			name: "ignores verify unmet when default verify command exists",
			input: TransitionCheckInput{
				Tickets: map[string]tracker.Ticket{
					"sw-02": {Status: tracker.StatusTodo, Phase: 2},
				},
				RequireExplicitRole: false,
				RequireVerifyCmd:    true,
				DefaultVerifyCmd:    "go test ./...",
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := CollectTransitionUnmetConditions(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("unmet length = %d, want %d (got %#v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("unmet[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestFormatDecisionBlockReason(t *testing.T) {
	tests := []struct {
		name string
		dec  Decision
		want string
	}{
		{
			name: "uses fallback reason when empty",
			dec:  Decision{},
			want: "guardian policy requirements unmet",
		},
		{
			name: "returns reason when no unmet list",
			dec:  Decision{Reason: "blocked by policy"},
			want: "blocked by policy",
		},
		{
			name: "appends unmet conditions",
			dec:  Decision{Reason: "transition gate requirements unmet", Unmet: []string{"missing profile", "missing verify_cmd"}},
			want: "transition gate requirements unmet (unmet conditions: missing profile; missing verify_cmd)",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatDecisionBlockReason(tc.dec); got != tc.want {
				t.Fatalf("FormatDecisionBlockReason() = %q, want %q", got, tc.want)
			}
		})
	}
}
