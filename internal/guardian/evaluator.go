package guardian

import (
	"context"
	"fmt"
	"strings"
)

type StrictEvaluator struct{}

func NewStrictEvaluator() Evaluator { return StrictEvaluator{} }

const (
	ruleTicketHasRequiredFields = "ticket_has_required_fields"
	rulePromptHasVerifyCommand  = "prompt_has_verify_command"
	ruleUnknownGuardianEvent    = "unknown_guardian_event"
)

func (StrictEvaluator) Evaluate(_ context.Context, req Request) (Decision, error) {
	switch req.Event {
	case EventBeforeSpawn:
		if strings.TrimSpace(asString(req.Context["profile"])) == "" {
			return blocked(req, ruleTicketHasRequiredFields, "missing explicit role/profile"), nil
		}
		if strings.TrimSpace(asString(req.Context["verify_cmd"])) == "" {
			return blocked(req, ruleTicketHasRequiredFields, "missing verify_cmd"), nil
		}
		return allowed(req), nil
	case EventBeforeMarkDone:
		if v, ok := req.Context["verify_passed"].(bool); ok && !v {
			return blocked(req, rulePromptHasVerifyCommand, "verify gate failed"), nil
		}
		return allowed(req), nil
	case EventPhaseTransition, EventPostBuildDone:
		return allowed(req), nil
	default:
		return warned(req, ruleUnknownGuardianEvent, fmt.Sprintf("unknown guardian event: %s", req.Event)), nil
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func allowed(req Request) Decision {
	return Decision{
		Result: ResultAllow,
		Target: target(req),
	}
}

func blocked(req Request, ruleID, reason string) Decision {
	return Decision{
		Result: ResultBlock,
		RuleID: ruleID,
		Reason: reason,
		Target: target(req),
	}
}

func warned(req Request, ruleID, reason string) Decision {
	return Decision{
		Result: ResultWarn,
		RuleID: ruleID,
		Reason: reason,
		Target: target(req),
	}
}

func target(req Request) string {
	if ticketID := strings.TrimSpace(req.TicketID); ticketID != "" {
		return "ticket:" + ticketID
	}
	if runID := strings.TrimSpace(req.RunID); runID != "" {
		return "run:" + runID
	}
	return ""
}
