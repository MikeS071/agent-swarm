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
	Event    Event
	TicketID string
	RunID    string
	Phase    int
	Context  map[string]any
}

type Decision struct {
	Result       Result
	RuleID       string
	Reason       string
	Unmet        []string
	EvidencePath string
}

type Evaluator interface {
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

type NoopEvaluator struct{}

func (NoopEvaluator) Evaluate(_ context.Context, _ Request) (Decision, error) {
	return Decision{Result: ResultAllow}, nil
}
