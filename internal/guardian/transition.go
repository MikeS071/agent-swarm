package guardian

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

// TransitionCheckInput contains the ticket slice and requirements checked at transition gates.
type TransitionCheckInput struct {
	Tickets             map[string]tracker.Ticket
	RequireExplicitRole bool
	RequireVerifyCmd    bool
	DefaultVerifyCmd    string
}

// NextPhaseTickets returns tickets in the first phase after currentPhase.
func NextPhaseTickets(tr *tracker.Tracker, currentPhase int) map[string]tracker.Ticket {
	if tr == nil {
		return map[string]tracker.Ticket{}
	}
	for _, phase := range tr.PhaseNumbers() {
		if phase > currentPhase {
			return tr.TicketsByPhase(phase)
		}
	}
	return map[string]tracker.Ticket{}
}

// CollectTransitionUnmetConditions reports explicit unmet transition conditions per ticket.
func CollectTransitionUnmetConditions(input TransitionCheckInput) []string {
	if len(input.Tickets) == 0 {
		return nil
	}

	ids := make([]string, 0, len(input.Tickets))
	for id := range input.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	defaultVerify := strings.TrimSpace(input.DefaultVerifyCmd)
	unmet := make([]string, 0)
	for _, id := range ids {
		tk := input.Tickets[id]
		if tk.Status != tracker.StatusTodo {
			continue
		}
		if input.RequireExplicitRole && strings.TrimSpace(tk.Profile) == "" {
			unmet = append(unmet, fmt.Sprintf("ticket %s missing explicit role/profile", id))
		}
		if input.RequireVerifyCmd && strings.TrimSpace(tk.VerifyCmd) == "" && defaultVerify == "" {
			unmet = append(unmet, fmt.Sprintf("ticket %s missing verify_cmd", id))
		}
	}
	return unmet
}

// UnmetConditionsFromContext normalizes unmet conditions from guardian request context.
func UnmetConditionsFromContext(ctx map[string]any) []string {
	if len(ctx) == 0 {
		return nil
	}
	v, ok := ctx["unmet_conditions"]
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case []string:
		out := make([]string, 0, len(vv))
		for _, s := range vv {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			s, _ := item.(string)
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}

// FormatDecisionBlockReason returns a user-facing block reason with explicit unmet conditions.
func FormatDecisionBlockReason(dec Decision) string {
	reason := strings.TrimSpace(dec.Reason)
	if reason == "" {
		reason = "guardian policy requirements unmet"
	}
	if len(dec.Unmet) == 0 {
		return reason
	}
	return fmt.Sprintf("%s (unmet conditions: %s)", reason, strings.Join(dec.Unmet, "; "))
}
