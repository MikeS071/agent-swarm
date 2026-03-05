package guardian

import (
	"context"
	"testing"
)

func TestStrictEvaluatorPhaseTransition(t *testing.T) {
	eval := StrictEvaluator{}

	tests := []struct {
		name      string
		ctx       map[string]any
		wantRes   Result
		wantRule  string
		wantUnmet int
	}{
		{
			name:    "allow when no unmet transition conditions",
			ctx:     map[string]any{},
			wantRes: ResultAllow,
		},
		{
			name: "block when unmet transition conditions are present",
			ctx: map[string]any{
				"unmet_conditions": []string{"missing prd examples", "missing spec schema"},
			},
			wantRes:   ResultBlock,
			wantRule:  "transition_gate_requirements",
			wantUnmet: 2,
		},
		{
			name: "ignores non-string unmet condition values",
			ctx: map[string]any{
				"unmet_conditions": []any{"missing verify_cmd", 42, true},
			},
			wantRes:   ResultBlock,
			wantRule:  "transition_gate_requirements",
			wantUnmet: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dec, err := eval.Evaluate(context.Background(), Request{Event: EventPhaseTransition, Context: tc.ctx})
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if dec.Result != tc.wantRes {
				t.Fatalf("result = %s, want %s", dec.Result, tc.wantRes)
			}
			if tc.wantRule != "" && dec.RuleID != tc.wantRule {
				t.Fatalf("rule = %q, want %q", dec.RuleID, tc.wantRule)
			}
			if len(dec.Unmet) != tc.wantUnmet {
				t.Fatalf("unmet count = %d, want %d (%#v)", len(dec.Unmet), tc.wantUnmet, dec.Unmet)
			}
		})
	}
}

func TestStrictEvaluatorPostBuildTransition(t *testing.T) {
	eval := StrictEvaluator{}
	dec, err := eval.Evaluate(context.Background(), Request{Event: EventPostBuildDone, Context: map[string]any{"unmet_conditions": []string{"missing report"}}})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if dec.Result != ResultBlock {
		t.Fatalf("result = %s, want %s", dec.Result, ResultBlock)
	}
	if len(dec.Unmet) != 1 || dec.Unmet[0] != "missing report" {
		t.Fatalf("unexpected unmet conditions: %#v", dec.Unmet)
	}
}
