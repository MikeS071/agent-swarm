package tui

import (
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type runProgressSummary struct {
	RunID             string
	IntegrationStatus string
	PostBuildSteps    map[string]string
	PostBuildOrder    []string
	PostBuildDone     int
	PostBuildRunning  int
	PostBuildFailed   int
	PostBuildPending  int
	PostBuildSkipped  int
	PostBuildTotal    int
}

func postBuildOrder(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return normalizePostBuildOrder(cfg.PostBuild.Order)
}

func summarizeFeatureStats(tr *tracker.Tracker, order []string) tracker.Stats {
	var stats tracker.Stats
	if tr == nil {
		return stats
	}
	stepSet := makeStepSet(order)
	for id, tk := range tr.Tickets {
		if _, ok := postBuildStepForTicket(id, tk, stepSet); ok {
			continue
		}
		if isIntegrationTicket(id, tk, stepSet) {
			continue
		}
		stats.Total++
		switch strings.TrimSpace(tk.Status) {
		case tracker.StatusDone:
			stats.Done++
		case tracker.StatusRunning:
			stats.Running++
		case tracker.StatusTodo:
			stats.Todo++
		case tracker.StatusFailed:
			stats.Failed++
		case "blocked":
			stats.Blocked++
		}
	}
	return stats
}

func summarizeRunProgress(tr *tracker.Tracker, order []string) runProgressSummary {
	out := runProgressSummary{
		IntegrationStatus: "pending",
		PostBuildOrder:    normalizePostBuildOrder(order),
		PostBuildSteps:    map[string]string{},
	}
	for _, step := range out.PostBuildOrder {
		out.PostBuildSteps[step] = "pending"
	}
	out.PostBuildTotal = len(out.PostBuildOrder)
	if tr == nil {
		out.PostBuildPending = out.PostBuildTotal
		return out
	}

	stepSet := makeStepSet(out.PostBuildOrder)
	runID := chooseRunID(tr, stepSet)
	out.RunID = runID

	seenSteps := map[string]bool{}
	anyStepSeen := false
	for id, tk := range tr.Tickets {
		ticketRunID := strings.TrimSpace(tk.RunID)
		if runID != "" && ticketRunID != "" && ticketRunID != runID {
			continue
		}
		if isIntegrationTicket(id, tk, stepSet) {
			out.IntegrationStatus = mergeSuiteStatus(out.IntegrationStatus, ticketToSuiteStatus(tk.Status))
			continue
		}
		step, ok := postBuildStepForTicket(id, tk, stepSet)
		if !ok {
			continue
		}
		anyStepSeen = true
		seenSteps[step] = true
		out.PostBuildSteps[step] = mergeSuiteStatus(out.PostBuildSteps[step], ticketToSuiteStatus(tk.Status))
	}
	for _, step := range out.PostBuildOrder {
		if seenSteps[step] {
			continue
		}
		if anyStepSeen {
			out.PostBuildSteps[step] = "skipped"
		} else {
			out.PostBuildSteps[step] = "pending"
		}
	}

	for _, step := range out.PostBuildOrder {
		switch out.PostBuildSteps[step] {
		case "done":
			out.PostBuildDone++
		case "running":
			out.PostBuildRunning++
		case "failed":
			out.PostBuildFailed++
		case "skipped":
			out.PostBuildSkipped++
		default:
			out.PostBuildPending++
		}
	}
	return out
}

func chooseRunID(tr *tracker.Tracker, stepSet map[string]struct{}) string {
	if tr == nil {
		return ""
	}
	bestID := ""
	bestScore := -1
	for id, tk := range tr.Tickets {
		runID := strings.TrimSpace(tk.RunID)
		if runID == "" {
			continue
		}
		score := 0
		if isIntegrationTicket(id, tk, stepSet) {
			score += 100
		}
		if _, ok := postBuildStepForTicket(id, tk, stepSet); ok {
			score += 80
		}
		switch strings.TrimSpace(tk.Status) {
		case tracker.StatusRunning:
			score += 30
		case tracker.StatusTodo, "blocked":
			score += 20
		case tracker.StatusFailed:
			score += 10
		case tracker.StatusDone:
			score += 5
		}
		score += tk.Phase
		if score > bestScore || (score == bestScore && runID > bestID) {
			bestScore = score
			bestID = runID
		}
	}
	return bestID
}

func makeStepSet(order []string) map[string]struct{} {
	stepSet := make(map[string]struct{}, len(order))
	for _, step := range normalizePostBuildOrder(order) {
		stepSet[step] = struct{}{}
	}
	return stepSet
}

func normalizePostBuildOrder(order []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(order))
	for _, raw := range order {
		step := strings.TrimSpace(raw)
		if step == "" {
			continue
		}
		if _, ok := seen[step]; ok {
			continue
		}
		seen[step] = struct{}{}
		out = append(out, step)
	}
	if len(out) == 0 {
		return out
	}
	return out
}

func isIntegrationTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) bool {
	ticketType := strings.TrimSpace(tk.Type)
	if ticketType == "integration" {
		return true
	}
	if ticketType != "" {
		return false
	}
	if !strings.HasPrefix(strings.TrimSpace(id), "int-") {
		return false
	}
	if _, isStep := stepSet["int"]; isStep {
		return false
	}
	return true
}

func postBuildStepForTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) (string, bool) {
	ticketType := strings.TrimSpace(tk.Type)
	if ticketType != "" {
		_, ok := stepSet[ticketType]
		return ticketType, ok
	}
	rawID := strings.TrimSpace(id)
	i := strings.Index(rawID, "-")
	if i <= 0 || i+1 >= len(rawID) {
		return "", false
	}
	step := strings.TrimSpace(rawID[:i])
	_, ok := stepSet[step]
	return step, ok
}

func ticketToSuiteStatus(status string) string {
	switch strings.TrimSpace(status) {
	case tracker.StatusDone:
		return "done"
	case tracker.StatusRunning:
		return "running"
	case tracker.StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}

func mergeSuiteStatus(current, next string) string {
	rank := map[string]int{
		"failed":  5,
		"running": 4,
		"done":    3,
		"pending": 2,
		"skipped": 1,
	}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func formatPostBuildSteps(order []string, steps map[string]string) string {
	if len(order) == 0 {
		return ""
	}
	parts := make([]string, 0, len(order))
	for _, step := range order {
		status := strings.TrimSpace(steps[step])
		if status == "" {
			status = "pending"
		}
		parts = append(parts, step+"="+status)
	}
	sort.Strings(parts)
	return strings.Join(parts, " | ")
}
