package rules

import (
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

var requiredPromptSections = []string{
	"## objective",
	"## dependencies",
	"## scope",
	"## verify",
}

// MissingTicketDescFields returns required semantic fields missing from a ticket description.
func MissingTicketDescFields(desc string) []string {
	lower := strings.ToLower(strings.TrimSpace(desc))
	missing := make([]string, 0, 2)
	if !strings.Contains(lower, "scope") {
		missing = append(missing, "scope")
	}
	if !strings.Contains(lower, "verify") {
		missing = append(missing, "verify")
	}
	return missing
}

func TicketDescHasScopeAndVerify(desc string) bool {
	return len(MissingTicketDescFields(desc)) == 0
}

// MissingPromptSections returns required markdown section headers absent from prompt body.
func MissingPromptSections(prompt string) []string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	missing := make([]string, 0, len(requiredPromptSections))
	for _, sec := range requiredPromptSections {
		if !strings.Contains(lower, sec) {
			missing = append(missing, sec)
		}
	}
	return missing
}

func PromptHasRequiredSections(prompt string) bool {
	return len(MissingPromptSections(prompt)) == 0
}

type PhaseChainResult struct {
	Phase         int
	MissingKinds  []string
	GapWithoutInt []string
	TstWithoutGap []string
}

func (r PhaseChainResult) Valid() bool {
	return len(r.MissingKinds) == 0 && len(r.GapWithoutInt) == 0 && len(r.TstWithoutGap) == 0
}

// CheckPhaseIntGapTstChain validates that a phase has at least one int->gap->tst
// dependency chain. Ticket kind is taken from ticket.type when present, otherwise
// inferred from ticket id prefix (int-/gap-/tst-).
func CheckPhaseIntGapTstChain(tickets map[string]tracker.Ticket, phase int) PhaseChainResult {
	res := PhaseChainResult{Phase: phase}
	ints := make(map[string]struct{})
	gaps := make(map[string]tracker.Ticket)
	tsts := make(map[string]tracker.Ticket)

	for id, tk := range tickets {
		if tk.Phase != phase {
			continue
		}
		switch ticketKind(id, tk) {
		case "int":
			ints[id] = struct{}{}
		case "gap":
			gaps[id] = tk
		case "tst":
			tsts[id] = tk
		}
	}

	if len(ints) == 0 {
		res.MissingKinds = append(res.MissingKinds, "int")
	}
	if len(gaps) == 0 {
		res.MissingKinds = append(res.MissingKinds, "gap")
	}
	if len(tsts) == 0 {
		res.MissingKinds = append(res.MissingKinds, "tst")
	}
	if len(res.MissingKinds) > 0 {
		sort.Strings(res.MissingKinds)
		return res
	}

	validGaps := map[string]struct{}{}
	for id, tk := range gaps {
		if dependsOnAny(tk.Depends, ints) {
			validGaps[id] = struct{}{}
			continue
		}
		res.GapWithoutInt = append(res.GapWithoutInt, id)
	}
	if len(validGaps) == 0 {
		sort.Strings(res.GapWithoutInt)
		return res
	}

	for id, tk := range tsts {
		if dependsOnAny(tk.Depends, validGaps) {
			continue
		}
		res.TstWithoutGap = append(res.TstWithoutGap, id)
	}
	sort.Strings(res.GapWithoutInt)
	sort.Strings(res.TstWithoutGap)
	return res
}

func ticketKind(id string, tk tracker.Ticket) string {
	if v := strings.ToLower(strings.TrimSpace(tk.Type)); v != "" {
		return v
	}
	lid := strings.ToLower(strings.TrimSpace(id))
	switch {
	case strings.HasPrefix(lid, "int-"):
		return "int"
	case strings.HasPrefix(lid, "gap-"):
		return "gap"
	case strings.HasPrefix(lid, "tst-"):
		return "tst"
	default:
		return ""
	}
}

func dependsOnAny(depends []string, ids map[string]struct{}) bool {
	for _, dep := range depends {
		if _, ok := ids[dep]; ok {
			return true
		}
	}
	return false
}
