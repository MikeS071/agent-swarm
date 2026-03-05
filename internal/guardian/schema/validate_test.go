package schema

import (
	"errors"
	"strings"
	"testing"
)

func validPolicyForValidation() *FlowPolicy {
	return &FlowPolicy{
		Version:           2,
		Mode:              ModeAdvisory,
		EnforcementPoints: []string{"before_spawn", "transition"},
		Rules: []Rule{
			{
				ID:                "rule-1",
				Enabled:           true,
				Description:       "validate ticket fields",
				Severity:          "block",
				EnforcementPoints: []string{"before_spawn"},
				Target: Target{
					Kind:   "ticket",
					Source: "swarm/tracker.json",
					Fields: []string{"objective", "scope", "verify"},
				},
				Check: Check{
					Type:   "ticket_fields",
					Params: map[string]any{"required_fields": []string{"objective", "scope", "verify"}},
				},
				PassWhen: PassWhen{
					Op: "all",
					Conditions: []Condition{
						{Metric: "required_fields_present", Equals: true},
					},
				},
				FailReason: "missing fields",
				Evidence: EvidenceRef{
					Kind: "json",
					Path: "evidence/rule-1.json",
				},
			},
		},
		Events: Events{
			File:    "guardian-events.jsonl",
			Include: []string{"timestamp", "result"},
		},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(p *FlowPolicy)
		wantErr     bool
		errContains []string
	}{
		{
			name: "valid policy",
		},
		{
			name: "invalid top-level fields",
			mutate: func(p *FlowPolicy) {
				p.Version = 1
				p.Mode = "strict"
				p.EnforcementPoints = nil
			},
			wantErr:     true,
			errContains: []string{"version", "mode", "enforcement_points"},
		},
		{
			name: "invalid rule semantics",
			mutate: func(p *FlowPolicy) {
				p.Rules[0].ID = ""
				p.Rules[0].Severity = "critical"
				p.Rules[0].Target = Target{Kind: "file", Paths: nil, Match: "invalid"}
				p.Rules[0].Check = Check{}
				p.Rules[0].PassWhen = PassWhen{}
				p.Rules[0].FailReason = ""
				p.Rules[0].Evidence.Path = ""
			},
			wantErr: true,
			errContains: []string{
				"rules[0].id",
				"rules[0].severity",
				"rules[0].target.paths",
				"rules[0].target.match",
				"rules[0].check.type",
				"rules[0].pass_when.op",
				"rules[0].pass_when.conditions",
				"rules[0].fail_reason",
				"rules[0].evidence.path",
			},
		},
		{
			name: "nil policy",
			mutate: func(p *FlowPolicy) {
				_ = p
			},
			wantErr:     true,
			errContains: []string{"policy"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var p *FlowPolicy
			if tc.name == "nil policy" {
				p = nil
			} else {
				p = validPolicyForValidation()
				if tc.mutate != nil {
					tc.mutate(p)
				}
			}

			err := Validate(p)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var verrs ValidationErrors
				if !errors.As(err, &verrs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
				for _, part := range tc.errContains {
					if !strings.Contains(err.Error(), part) {
						t.Fatalf("error %q missing %q", err, part)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
