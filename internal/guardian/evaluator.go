package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/guardian/rules"
)

type StrictEvaluator struct{}

func NewStrictEvaluator() Evaluator { return StrictEvaluator{} }

func (StrictEvaluator) Evaluate(_ context.Context, req Request) (Decision, error) {
	switch req.Event {
	case EventBeforeSpawn:
		mode := guardianMode(req)
		in := rules.Input{
			TicketID:            req.TicketID,
			Profile:             strings.TrimSpace(asString(req.Context["profile"])),
			PromptPath:          strings.TrimSpace(asString(req.Context["prompt_path"])),
			PromptBody:          asString(req.Context["prompt_body"]),
			VerifyCmd:           strings.TrimSpace(asString(req.Context["verify_cmd_effective"])),
			RequireExplicitRole: asBool(req.Context["require_explicit_role"]),
			RequireVerifyCmd:    asBool(req.Context["require_verify_cmd"]),
		}
		violations := rules.EvaluateBeforeSpawn(in)
		if len(violations) == 0 {
			return Decision{Result: ResultAllow}, nil
		}

		evidenceDir := strings.TrimSpace(asString(req.Context["evidence_dir"]))
		out := make([]Violation, 0, len(violations))
		for _, v := range violations {
			evidencePath := writeEvidence(evidenceDir, req, v)
			out = append(out, Violation{RuleID: v.RuleID, Reason: v.Reason, EvidencePath: evidencePath})
		}

		result := ResultBlock
		if mode == "advisory" {
			result = ResultWarn
		}
		first := out[0]
		return Decision{
			Result:       result,
			RuleID:       first.RuleID,
			Reason:       first.Reason,
			EvidencePath: first.EvidencePath,
			Violations:   out,
		}, nil
	case EventBeforeMarkDone:
		if v, ok := req.Context["verify_passed"].(bool); ok && !v {
			return Decision{Result: ResultBlock, RuleID: rules.RulePromptHasVerifyCommand, Reason: "verify gate failed"}, nil
		}
		return Decision{Result: ResultAllow}, nil
	case EventPhaseTransition, EventPostBuildDone:
		return Decision{Result: ResultAllow}, nil
	default:
		return Decision{Result: ResultWarn, Reason: fmt.Sprintf("unknown guardian event: %s", req.Event)}, nil
	}
}

func guardianMode(req Request) string {
	mode := strings.ToLower(strings.TrimSpace(asString(req.Context["guardian_mode"])))
	switch mode {
	case "advisory", "enforce":
		return mode
	default:
		return "enforce"
	}
}

func writeEvidence(dir string, req Request, violation rules.Violation) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := fmt.Sprintf("%s-%d.json", sanitizeToken(violation.RuleID), time.Now().UnixNano())
	path := filepath.Join(dir, name)
	payload := map[string]any{
		"ticket_id":    req.TicketID,
		"event":        req.Event,
		"run_id":       req.RunID,
		"phase":        req.Phase,
		"rule_id":      violation.RuleID,
		"reason":       violation.Reason,
		"details":      violation.Evidence,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return ""
	}
	return path
}

func sanitizeToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "rule"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "rule"
	}
	return out
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}
