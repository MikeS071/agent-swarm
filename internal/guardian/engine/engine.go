package engine

import "time"

type Mode string

const (
	ModeAdvisory Mode = "advisory"
	ModeEnforce  Mode = "enforce"
)

type Result string

const (
	ResultAllow Result = "ALLOW"
	ResultWarn  Result = "WARN"
	ResultBlock Result = "BLOCK"
)

type Check struct {
	Rule     string
	Passed   bool
	Reason   string
	Target   string
	Evidence string
}

// Decision is a normalized guardian policy outcome.
type Decision struct {
	Result   Result    `json:"result"`
	Rule     string    `json:"rule"`
	Reason   string    `json:"reason,omitempty"`
	Target   string    `json:"target,omitempty"`
	Evidence string    `json:"evidence,omitempty"`
	Time     time.Time `json:"time"`
}

// Evaluate converts low-level check outcomes into guardian decisions.
func Evaluate(mode Mode, checks []Check) []Decision {
	if mode != ModeEnforce {
		mode = ModeAdvisory
	}

	out := make([]Decision, 0, len(checks))
	for _, check := range checks {
		result := ResultAllow
		if !check.Passed {
			if mode == ModeEnforce {
				result = ResultBlock
			} else {
				result = ResultWarn
			}
		}

		reason := check.Reason
		if reason == "" {
			if check.Passed {
				reason = "check passed"
			} else {
				reason = "check failed"
			}
		}

		out = append(out, Decision{
			Result:   result,
			Rule:     check.Rule,
			Reason:   reason,
			Target:   check.Target,
			Evidence: check.Evidence,
			Time:     time.Now().UTC(),
		})
	}
	return out
}

// Overall collapses many decisions into a single result using BLOCK > WARN > ALLOW precedence.
func Overall(decisions []Decision) Result {
	overall := ResultAllow
	for _, d := range decisions {
		switch d.Result {
		case ResultBlock:
			return ResultBlock
		case ResultWarn:
			overall = ResultWarn
		}
	}
	return overall
}
