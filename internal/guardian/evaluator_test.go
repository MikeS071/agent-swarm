package guardian

import (
	"context"
	"encoding/json"
	"testing"
)

func TestStrictEvaluatorEvaluate_BeforeSpawn(t *testing.T) {
	t.Parallel()

	ev := NewStrictEvaluator()
	tests := []struct {
		name     string
		req      Request
		expected Decision
	}{
		{
			name: "blocks when profile is missing",
			req: Request{
				Event:    EventBeforeSpawn,
				TicketID: "g1-02",
				Context: map[string]any{
					"verify_cmd": "go test ./...",
				},
			},
			expected: Decision{
				Result: ResultBlock,
				RuleID: "ticket_has_required_fields",
				Reason: "missing explicit role/profile",
				Target: "ticket:g1-02",
			},
		},
		{
			name: "blocks when verify command is missing",
			req: Request{
				Event:    EventBeforeSpawn,
				TicketID: "g1-02",
				Context: map[string]any{
					"profile": "code-agent",
				},
			},
			expected: Decision{
				Result: ResultBlock,
				RuleID: "ticket_has_required_fields",
				Reason: "missing verify_cmd",
				Target: "ticket:g1-02",
			},
		},
		{
			name: "allows when required fields are present",
			req: Request{
				Event:    EventBeforeSpawn,
				TicketID: "g1-02",
				Context: map[string]any{
					"profile":    "code-agent",
					"verify_cmd": "go test ./internal/guardian/... -run Evaluator",
				},
			},
			expected: Decision{
				Result: ResultAllow,
				Target: "ticket:g1-02",
			},
		},
		{
			name: "edge case nil context is handled deterministically",
			req: Request{
				Event:    EventBeforeSpawn,
				TicketID: "g1-02",
				Context:  nil,
			},
			expected: Decision{
				Result: ResultBlock,
				RuleID: "ticket_has_required_fields",
				Reason: "missing explicit role/profile",
				Target: "ticket:g1-02",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ev.Evaluate(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if got != tt.expected {
				t.Fatalf("Evaluate() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestStrictEvaluatorEvaluate_BeforeMarkDone(t *testing.T) {
	t.Parallel()

	ev := NewStrictEvaluator()
	tests := []struct {
		name     string
		req      Request
		expected Decision
	}{
		{
			name: "blocks when verify gate fails",
			req: Request{
				Event:    EventBeforeMarkDone,
				TicketID: "g1-02",
				Context: map[string]any{
					"verify_passed": false,
				},
			},
			expected: Decision{
				Result: ResultBlock,
				RuleID: "prompt_has_verify_command",
				Reason: "verify gate failed",
				Target: "ticket:g1-02",
			},
		},
		{
			name: "allows when verify gate passes",
			req: Request{
				Event:    EventBeforeMarkDone,
				TicketID: "g1-02",
				Context: map[string]any{
					"verify_passed": true,
				},
			},
			expected: Decision{
				Result: ResultAllow,
				Target: "ticket:g1-02",
			},
		},
		{
			name: "edge case non bool verify flag defaults to allow",
			req: Request{
				Event:    EventBeforeMarkDone,
				TicketID: "g1-02",
				Context: map[string]any{
					"verify_passed": "false",
				},
			},
			expected: Decision{
				Result: ResultAllow,
				Target: "ticket:g1-02",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ev.Evaluate(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if got != tt.expected {
				t.Fatalf("Evaluate() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestStrictEvaluatorEvaluate_WarnAndDeterministic(t *testing.T) {
	t.Parallel()

	ev := NewStrictEvaluator()
	req := Request{
		Event:    Event("unknown_event"),
		TicketID: "g1-02",
	}

	first, err := ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	second, err := ev.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	want := Decision{
		Result: ResultWarn,
		RuleID: "unknown_guardian_event",
		Reason: "unknown guardian event: unknown_event",
		Target: "ticket:g1-02",
	}

	if first != want {
		t.Fatalf("first decision = %#v, want %#v", first, want)
	}
	if second != want {
		t.Fatalf("second decision = %#v, want %#v", second, want)
	}
}

func TestDecisionJSONContract(t *testing.T) {
	t.Parallel()

	dec := Decision{
		Result:       ResultBlock,
		RuleID:       "ticket_has_required_fields",
		Reason:       "missing verify_cmd",
		Target:       "ticket:g1-02",
		EvidencePath: "/tmp/evidence.json",
	}

	raw, err := json.Marshal(dec)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if parsed["result"] != string(ResultBlock) {
		t.Fatalf("result = %v, want %q", parsed["result"], ResultBlock)
	}
	if parsed["rule"] != dec.RuleID {
		t.Fatalf("rule = %v, want %q", parsed["rule"], dec.RuleID)
	}
	if parsed["reason"] != dec.Reason {
		t.Fatalf("reason = %v, want %q", parsed["reason"], dec.Reason)
	}
	if parsed["target"] != dec.Target {
		t.Fatalf("target = %v, want %q", parsed["target"], dec.Target)
	}
	if parsed["evidence"] != dec.EvidencePath {
		t.Fatalf("evidence = %v, want %q", parsed["evidence"], dec.EvidencePath)
	}
}
