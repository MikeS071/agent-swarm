package guardian

import (
	"context"
	"fmt"
	"strings"
)

type StrictEvaluator struct{}

func NewStrictEvaluator() Evaluator { return StrictEvaluator{} }

func (StrictEvaluator) Evaluate(_ context.Context, req Request) (Decision, error) {
	switch req.Event {
	case EventBeforeSpawn:
		if strings.TrimSpace(asString(req.Context["profile"])) == "" {
			return Decision{Result: ResultBlock, RuleID: "ticket_has_required_fields", Reason: "missing explicit role/profile"}, nil
		}
		if strings.TrimSpace(asString(req.Context["verify_cmd"])) == "" {
			return Decision{Result: ResultBlock, RuleID: "ticket_has_required_fields", Reason: "missing verify_cmd"}, nil
		}
		return Decision{Result: ResultAllow}, nil
	case EventBeforeMarkDone:
		if v, ok := req.Context["verify_passed"].(bool); ok && !v {
			return Decision{Result: ResultBlock, RuleID: "prompt_has_verify_command", Reason: "verify gate failed"}, nil
		}
		return Decision{Result: ResultAllow}, nil
	case EventPhaseTransition, EventPostBuildDone:
		unmet := UnmetConditionsFromContext(req.Context)
		if len(unmet) > 0 {
			ruleID := "transition_gate_requirements"
			reason := "transition gate requirements unmet"
			if req.Event == EventPostBuildDone {
				ruleID = "post_build_transition_requirements"
				reason = "post-build completion requirements unmet"
			}
			return Decision{Result: ResultBlock, RuleID: ruleID, Reason: reason, Unmet: unmet}, nil
		}
		return Decision{Result: ResultAllow}, nil
	default:
		return Decision{Result: ResultWarn, Reason: fmt.Sprintf("unknown guardian event: %s", req.Event)}, nil
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
