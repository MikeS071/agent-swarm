package server

import (
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type runProgressPayload struct {
	RunID             string            `json:"run_id,omitempty"`
	IntegrationStatus string            `json:"integration_status"`
	PostBuildSteps    map[string]string `json:"post_build_steps"`
	PostBuildDone     int               `json:"post_build_done"`
	PostBuildRunning  int               `json:"post_build_running"`
	PostBuildFailed   int               `json:"post_build_failed"`
	PostBuildPending  int               `json:"post_build_pending"`
	PostBuildSkipped  int               `json:"post_build_skipped"`
	PostBuildTotal    int               `json:"post_build_total"`
}

func serverPostBuildOrder(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	return normalizeOrder(cfg.PostBuild.Order)
}

func featureStatsFromTracker(tr *tracker.Tracker, order []string) tracker.Stats {
	var stats tracker.Stats
	if tr == nil {
		return stats
	}
	stepSet := orderSet(order)
	for id, tk := range tr.Tickets {
		if _, ok := serverPostBuildStepForTicket(id, tk, stepSet); ok {
			continue
		}
		if serverIsIntegrationTicket(id, tk, stepSet) {
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

func runProgressFromTracker(tr *tracker.Tracker, order []string) runProgressPayload {
	normalizedOrder := normalizeOrder(order)
	out := runProgressPayload{
		IntegrationStatus: "pending",
		PostBuildSteps:    map[string]string{},
		PostBuildTotal:    len(normalizedOrder),
	}
	for _, step := range normalizedOrder {
		out.PostBuildSteps[step] = "pending"
	}
	if tr == nil {
		out.PostBuildPending = out.PostBuildTotal
		return out
	}

	stepSet := orderSet(normalizedOrder)
	runID := serverChooseRunID(tr, stepSet)
	out.RunID = runID

	seen := map[string]bool{}
	anyStep := false
	for id, tk := range tr.Tickets {
		ticketRunID := strings.TrimSpace(tk.RunID)
		if runID != "" && ticketRunID != "" && ticketRunID != runID {
			continue
		}
		if serverIsIntegrationTicket(id, tk, stepSet) {
			out.IntegrationStatus = serverMergeStatus(out.IntegrationStatus, serverTicketToSuiteStatus(tk.Status))
			continue
		}
		step, ok := serverPostBuildStepForTicket(id, tk, stepSet)
		if !ok {
			continue
		}
		seen[step] = true
		anyStep = true
		out.PostBuildSteps[step] = serverMergeStatus(out.PostBuildSteps[step], serverTicketToSuiteStatus(tk.Status))
	}
	for _, step := range normalizedOrder {
		if seen[step] {
			continue
		}
		if anyStep {
			out.PostBuildSteps[step] = "skipped"
		} else {
			out.PostBuildSteps[step] = "pending"
		}
	}
	for _, step := range normalizedOrder {
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

func serverChooseRunID(tr *tracker.Tracker, stepSet map[string]struct{}) string {
	bestID := ""
	bestScore := -1
	for id, tk := range tr.Tickets {
		runID := strings.TrimSpace(tk.RunID)
		if runID == "" {
			continue
		}
		score := 0
		if serverIsIntegrationTicket(id, tk, stepSet) {
			score += 100
		}
		if _, ok := serverPostBuildStepForTicket(id, tk, stepSet); ok {
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
			bestID = runID
			bestScore = score
		}
	}
	return bestID
}

func normalizeOrder(order []string) []string {
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
	return out
}

func orderSet(order []string) map[string]struct{} {
	set := make(map[string]struct{}, len(order))
	for _, step := range order {
		set[step] = struct{}{}
	}
	return set
}

func serverIsIntegrationTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) bool {
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

func serverPostBuildStepForTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) (string, bool) {
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

func serverTicketToSuiteStatus(status string) string {
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

func serverMergeStatus(current, next string) string {
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
