package guardian

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/guardian/rules"
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
		desc := asString(req.Context["desc"])
		if !rules.TicketDescHasScopeAndVerify(desc) {
			return Decision{Result: ResultBlock, RuleID: "ticket_desc_has_scope_and_verify", Reason: "ticket description missing scope/verify"}, nil
		}
		prompt := asString(req.Context["prompt"])
		if strings.TrimSpace(prompt) == "" {
			return Decision{Result: ResultBlock, RuleID: "prompt_template_sections", Reason: "missing prompt template"}, nil
		}
		if !rules.PromptHasRequiredSections(prompt) {
			return Decision{Result: ResultBlock, RuleID: "prompt_template_sections", Reason: "prompt missing required sections"}, nil
		}
		return Decision{Result: ResultAllow}, nil
	case EventBeforeMarkDone:
		if v, ok := req.Context["verify_passed"].(bool); ok && !v {
			return Decision{Result: ResultBlock, RuleID: "prompt_has_verify_command", Reason: "verify gate failed"}, nil
		}
		return Decision{Result: ResultAllow}, nil
	case EventPhaseTransition, EventPostBuildDone:
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
