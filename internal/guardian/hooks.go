package guardian

import "context"

type Result string

const (
	ResultAllow Result = "ALLOW"
	ResultWarn  Result = "WARN"
	ResultBlock Result = "BLOCK"
)

type Event string

const (
	EventBeforeSpawn     Event = "before_spawn"
	EventBeforeMarkDone  Event = "before_mark_done"
	EventPhaseTransition Event = "phase_transition"
	EventPostBuildDone   Event = "post_build_complete"
)

type Request struct {
	Event    Event          `json:"event"`
	TicketID string         `json:"ticket_id,omitempty"`
	RunID    string         `json:"run_id,omitempty"`
	Phase    int            `json:"phase,omitempty"`
	Context  map[string]any `json:"context,omitempty"`
}

type Decision struct {
	Result       Result `json:"result"`
	RuleID       string `json:"rule,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Target       string `json:"target,omitempty"`
	EvidencePath string `json:"evidence,omitempty"`
}

type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

type NoopEvaluator struct{}

func (NoopEvaluator) Evaluate(_ context.Context, _ Request) (Decision, error) {
	return Decision{Result: ResultAllow}, nil
}
