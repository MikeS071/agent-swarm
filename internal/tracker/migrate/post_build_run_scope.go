package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultLegacyRunID = "legacy-run-1"

var defaultLegacyPostBuildSteps = []string{"int", "gap", "tst", "review", "sec", "doc", "clean", "mem"}

type PostBuildRunScopeOptions struct {
	Steps []string
	RunID string
}

type PostBuildRunScopeResult struct {
	LegacyDetected         bool
	RunID                  string
	LegacyTicketCount      int
	RemovedTicketIDs       []string
	RemovedDependencyCount int
	UpdatedTicketCount     int
	IntegrationStatus      string
	PostBuildStatuses      map[string]string
}

func PreviewPostBuildRunScope(path string, opts PostBuildRunScopeOptions) (PostBuildRunScopeResult, error) {
	res, _, err := migratePostBuildRunScope(path, opts, false)
	return res, err
}

func ApplyPostBuildRunScope(path string, opts PostBuildRunScopeOptions) (PostBuildRunScopeResult, error) {
	res, _, err := migratePostBuildRunScope(path, opts, true)
	return res, err
}

func migratePostBuildRunScope(path string, opts PostBuildRunScopeOptions, apply bool) (PostBuildRunScopeResult, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PostBuildRunScopeResult{}, nil, fmt.Errorf("read tracker %s: %w", path, err)
	}

	res, out, err := transformPostBuildRunScope(data, opts)
	if err != nil {
		return PostBuildRunScopeResult{}, nil, fmt.Errorf("transform tracker %s: %w", path, err)
	}
	if apply && res.LegacyDetected {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return PostBuildRunScopeResult{}, nil, fmt.Errorf("write tracker %s: %w", path, err)
		}
	}
	return res, out, nil
}

func transformPostBuildRunScope(data []byte, opts PostBuildRunScopeOptions) (PostBuildRunScopeResult, []byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return PostBuildRunScopeResult{}, nil, fmt.Errorf("parse tracker JSON: %w", err)
	}

	tickets, err := requireObject(doc, "tickets")
	if err != nil {
		return PostBuildRunScopeResult{}, nil, err
	}

	steps := normalizeSteps(opts.Steps)
	stepSet := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		stepSet[step] = struct{}{}
	}
	integrationStep := pickIntegrationStep(steps)
	postBuildSteps := make([]string, 0, len(steps))
	for _, step := range steps {
		if step == integrationStep {
			continue
		}
		postBuildSteps = append(postBuildSteps, step)
	}

	ticketIDs := sortedObjectKeys(tickets)
	legacyIDs := make([]string, 0)
	legacyByStep := map[string][]string{}
	runIDCandidates := map[string]struct{}{}

	for _, id := range ticketIDs {
		rawTicket, err := requireTicketObject(tickets, id)
		if err != nil {
			return PostBuildRunScopeResult{}, nil, err
		}
		typeName := toString(rawTicket["type"])
		feature := toString(rawTicket["feature"])
		if step, _, ok := detectLegacyTicket(id, typeName, feature, stepSet); ok {
			legacyIDs = append(legacyIDs, id)
			legacyByStep[step] = append(legacyByStep[step], normalizeStatus(toString(rawTicket["status"])))
			continue
		}
		runID := strings.TrimSpace(toString(rawTicket["run_id"]))
		if runID != "" {
			runIDCandidates[runID] = struct{}{}
		}
	}

	if len(legacyIDs) == 0 {
		res := PostBuildRunScopeResult{LegacyDetected: false, PostBuildStatuses: map[string]string{}}
		return res, data, nil
	}
	legacySet := make(map[string]struct{}, len(legacyIDs))
	for _, id := range legacyIDs {
		legacySet[id] = struct{}{}
	}

	runID := chooseRunID(doc, opts.RunID, runIDCandidates)
	integrationStatus := aggregateStatus(legacyByStep[integrationStep])
	postBuildStatuses := make(map[string]string, len(postBuildSteps))
	for _, step := range postBuildSteps {
		postBuildStatuses[step] = aggregateStatus(legacyByStep[step])
	}

	runs, err := optionalObject(doc, "runs")
	if err != nil {
		return PostBuildRunScopeResult{}, nil, err
	}
	if runs == nil {
		runs = map[string]any{}
	}

	runStateRaw, exists := runs[runID]
	runState := map[string]any{}
	if exists {
		runState, err = asObject(runStateRaw)
		if err != nil {
			return PostBuildRunScopeResult{}, nil, fmt.Errorf("field runs[%q] must be an object", runID)
		}
	}
	runState["integration"] = map[string]any{"status": integrationStatus}
	postBuild := map[string]any{}
	if raw, ok := runState["postBuild"]; ok {
		postBuild, err = asObject(raw)
		if err != nil {
			return PostBuildRunScopeResult{}, nil, fmt.Errorf("field runs[%q].postBuild must be an object", runID)
		}
	}
	for _, step := range postBuildSteps {
		postBuild[step] = map[string]any{"status": postBuildStatuses[step]}
	}
	runState["postBuild"] = postBuild
	runs[runID] = runState
	doc["runs"] = runs
	doc["currentRunId"] = runID

	removedDeps := 0
	updatedTickets := 0
	for _, id := range ticketIDs {
		if _, legacy := legacySet[id]; legacy {
			delete(tickets, id)
			continue
		}
		rawTicket, err := requireTicketObject(tickets, id)
		if err != nil {
			return PostBuildRunScopeResult{}, nil, err
		}
		deps, hasDepends, err := readDepends(rawTicket)
		if err != nil {
			return PostBuildRunScopeResult{}, nil, fmt.Errorf("ticket %q: %w", id, err)
		}
		if !hasDepends {
			continue
		}
		filtered := make([]string, 0, len(deps))
		for _, dep := range deps {
			if _, removed := legacySet[dep]; removed {
				removedDeps++
				continue
			}
			filtered = append(filtered, dep)
		}
		if len(filtered) != len(deps) {
			rawTicket["depends"] = filtered
			updatedTickets++
		}
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return PostBuildRunScopeResult{}, nil, fmt.Errorf("marshal transformed tracker: %w", err)
	}

	res := PostBuildRunScopeResult{
		LegacyDetected:         true,
		RunID:                  runID,
		LegacyTicketCount:      len(legacyIDs),
		RemovedTicketIDs:       append([]string(nil), legacyIDs...),
		RemovedDependencyCount: removedDeps,
		UpdatedTicketCount:     updatedTickets,
		IntegrationStatus:      integrationStatus,
		PostBuildStatuses:      postBuildStatuses,
	}
	return res, out, nil
}

func FormatPostBuildRunScopeSummary(res PostBuildRunScopeResult, applied bool) string {
	if !res.LegacyDetected {
		return "No legacy per-feature post-build graph detected. No changes required."
	}

	mode := "dry-run"
	if applied {
		mode = "applied"
	}

	steps := make([]string, 0, len(res.PostBuildStatuses))
	for step := range res.PostBuildStatuses {
		steps = append(steps, step)
	}
	sort.Strings(steps)
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, fmt.Sprintf("%s=%s", step, res.PostBuildStatuses[step]))
	}

	return fmt.Sprintf(
		"post-build-run-scope migration (%s)\nrun_id: %s\nlegacy tickets: %d\nremoved dependencies: %d across %d tickets\nintegration: %s\npost_build: %s",
		mode,
		res.RunID,
		res.LegacyTicketCount,
		res.RemovedDependencyCount,
		res.UpdatedTicketCount,
		res.IntegrationStatus,
		strings.Join(parts, ", "),
	)
}

func requireObject(doc map[string]any, key string) (map[string]any, error) {
	raw, ok := doc[key]
	if !ok {
		return nil, fmt.Errorf("tracker missing %q", key)
	}
	obj, err := asObject(raw)
	if err != nil {
		return nil, fmt.Errorf("field %q must be an object", key)
	}
	return obj, nil
}

func optionalObject(doc map[string]any, key string) (map[string]any, error) {
	raw, ok := doc[key]
	if !ok || raw == nil {
		return nil, nil
	}
	obj, err := asObject(raw)
	if err != nil {
		return nil, fmt.Errorf("field %q must be an object", key)
	}
	return obj, nil
}

func requireTicketObject(tickets map[string]any, id string) (map[string]any, error) {
	raw, ok := tickets[id]
	if !ok {
		return nil, fmt.Errorf("ticket %q not found", id)
	}
	obj, err := asObject(raw)
	if err != nil {
		return nil, fmt.Errorf("ticket %q must be an object", id)
	}
	return obj, nil
}

func asObject(raw any) (map[string]any, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not an object")
	}
	return obj, nil
}

func toString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func detectLegacyTicket(id, ticketType, feature string, stepSet map[string]struct{}) (step, parsedFeature string, ok bool) {
	ticketType = strings.TrimSpace(ticketType)
	feature = strings.TrimSpace(feature)
	if ticketType != "" {
		if _, isStep := stepSet[ticketType]; isStep && feature != "" {
			return ticketType, feature, true
		}
	}

	i := strings.Index(id, "-")
	if i <= 0 || i+1 >= len(id) {
		return "", "", false
	}
	step = strings.TrimSpace(id[:i])
	parsedFeature = strings.TrimSpace(id[i+1:])
	if step == "" || parsedFeature == "" {
		return "", "", false
	}
	if _, isStep := stepSet[step]; !isStep {
		return "", "", false
	}
	return step, parsedFeature, true
}

func readDepends(ticket map[string]any) ([]string, bool, error) {
	raw, ok := ticket["depends"]
	if !ok || raw == nil {
		return nil, false, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("field depends must be an array")
	}
	deps := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false, fmt.Errorf("field depends[%d] must be a string", i)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		deps = append(deps, s)
	}
	return deps, true, nil
}

func normalizeSteps(steps []string) []string {
	if len(steps) == 0 {
		steps = defaultLegacyPostBuildSteps
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(steps))
	for _, raw := range steps {
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
		return append([]string(nil), defaultLegacyPostBuildSteps...)
	}
	return out
}

func pickIntegrationStep(steps []string) string {
	for _, step := range steps {
		if step == "int" {
			return step
		}
	}
	return "int"
}

func chooseRunID(doc map[string]any, explicit string, candidates map[string]struct{}) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit
	}
	if existing := strings.TrimSpace(toString(doc["currentRunId"])); existing != "" {
		return existing
	}
	if len(candidates) > 0 {
		ids := make([]string, 0, len(candidates))
		for id := range candidates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids[0]
	}
	return defaultLegacyRunID
}

func aggregateStatus(statuses []string) string {
	if len(statuses) == 0 {
		return "todo"
	}

	has := map[string]bool{}
	allDone := true
	for _, raw := range statuses {
		status := normalizeStatus(raw)
		has[status] = true
		if status != "done" {
			allDone = false
		}
	}
	if has["failed"] {
		return "failed"
	}
	if allDone {
		return "done"
	}
	if has["running"] {
		return "running"
	}
	if has["blocked"] {
		return "blocked"
	}
	if has["done"] {
		return "running"
	}
	return "todo"
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "done":
		return "done"
	case "running":
		return "running"
	case "failed":
		return "failed"
	case "blocked":
		return "blocked"
	default:
		return "todo"
	}
}

func sortedObjectKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
