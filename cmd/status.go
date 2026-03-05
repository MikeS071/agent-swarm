package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/tui"
	"github.com/MikeS071/agent-swarm/internal/watchdog"
	"github.com/MikeS071/agent-swarm/internal/worktree"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusProject string
var statusJSON bool
var statusWatch bool
var statusCompact bool
var statusLive bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tracker status",
	RunE: func(cmd *cobra.Command, args []string) error {
		if statusWatch {
			return tui.Run(cfgFile, statusProject, statusCompact)
		}

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := loadTrackerWithFallback(cfg, trackerPath)
		if err != nil {
			return err
		}
		if statusProject != "" && statusProject != tr.Project {
			return fmt.Errorf("project %q not found (tracker project is %q)", statusProject, tr.Project)
		}
		if !statusWatch && !statusLive {
			if be, err := buildBackend(cfg); err == nil {
				d := dispatcher.New(cfg, tr)
				wt := worktree.New(cfg.Project.Repo, "", cfg.Project.BaseBranch)
				wd := watchdog.New(cfg, tr, d, be, wt, buildNotifier(cfg))
				wd.SetConfigPath(cfgFile)
				if cfg.Guardian.Enabled {
					wd.SetGuardian(guardian.NewStrictEvaluator())
				}
				_ = wd.ReconcileRunning(cmd.Context())
			}
		}
		if statusLive {
			return runLiveStatus(cfgFile, cfg)
		}
		if statusJSON {
			return printStatusJSON(cfg, tr)
		}
		if statusCompact {
			printStatusCompact(cfg, tr)
			return nil
		}
		printStatusTable(cfg, tr)
		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&statusProject, "project", "", "project name")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output json")
	statusCmd.Flags().BoolVar(&statusWatch, "watch", false, "run live dashboard")
	statusCmd.Flags().BoolVar(&statusCompact, "compact", false, "compact output")
	statusCmd.Flags().BoolVar(&statusLive, "live", false, "live updating display (like top)")
	rootCmd.AddCommand(statusCmd)
}

func printStatusJSON(cfg *config.Config, tr *tracker.Tracker) error {
	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	tickets := make(map[string]tracker.Ticket, len(ids))
	for _, id := range ids {
		tickets[id] = tr.Tickets[id]
	}

	order := normalizedStatusOrder(nil)
	if cfg != nil {
		order = normalizedStatusOrder(cfg.PostBuild.Order)
	}
	featureStats := statusFeatureStats(tr, order)
	runProgress := statusRunProgressSummary(tr, order)

	payload := struct {
		Project      string                      `json:"project"`
		ActivePhase  int                         `json:"activePhase"`
		CurrentRunID string                      `json:"currentRunId,omitempty"`
		Runs         map[string]tracker.RunState `json:"runs,omitempty"`
		Stats        tracker.Stats               `json:"stats"`
		FeatureStats tracker.Stats               `json:"featureStats"`
		RunProgress  statusRunProgress           `json:"runProgress"`
		Spawnable    []string                    `json:"spawnable"`
		Tickets      map[string]tracker.Ticket   `json:"tickets"`
	}{
		Project:      tr.Project,
		ActivePhase:  tr.ActivePhase(),
		CurrentRunID: strings.TrimSpace(tr.CurrentRunID),
		Runs:         tr.Runs,
		Stats:        tr.Stats(),
		FeatureStats: featureStats,
		RunProgress:  runProgress,
		Spawnable:    tr.GetSpawnable(),
		Tickets:      tickets,
	}
	enc := json.NewEncoder(color.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printStatusTable(cfg *config.Config, tr *tracker.Tracker) {
	fmt.Fprintf(color.Output, "Project: %s\n", tr.Project)
	fmt.Fprintf(color.Output, "Active phase: %d\n", tr.ActivePhase())
	stats := tr.Stats()
	fmt.Fprintf(color.Output, "Stats: done=%d running=%d todo=%d failed=%d blocked=%d total=%d\n\n",
		stats.Done, stats.Running, stats.Todo, stats.Failed, stats.Blocked, stats.Total)
	order := normalizedStatusOrder(nil)
	if cfg != nil {
		order = normalizedStatusOrder(cfg.PostBuild.Order)
	}
	featureStats := statusFeatureStats(tr, order)
	runProgress := statusRunProgressSummary(tr, order)
	fmt.Fprintf(color.Output, "Feature stats: done=%d running=%d todo=%d failed=%d blocked=%d total=%d\n",
		featureStats.Done, featureStats.Running, featureStats.Todo, featureStats.Failed, featureStats.Blocked, featureStats.Total)
	fmt.Fprintf(color.Output, "Run progress: integration=%s post-build=%d/%d (running=%d failed=%d pending=%d skipped=%d)\n",
		runProgress.IntegrationStatus,
		runProgress.PostBuildDone,
		runProgress.PostBuildTotal,
		runProgress.PostBuildRunning,
		runProgress.PostBuildFailed,
		runProgress.PostBuildPending,
		runProgress.PostBuildSkipped,
	)
	if len(order) > 0 {
		fmt.Fprintf(color.Output, "Post-build: %s\n\n", statusPostBuildStepLine(order, runProgress.PostBuildSteps))
	}

	w := tabwriter.NewWriter(color.Output, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPHASE\tSTATUS\tBRANCH\tDEPENDS\tDESC")

	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := tr.Tickets[id]
		status := colorStatus(t.Status)
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", id, t.Phase, status, t.Branch, strings.Join(t.Depends, ","), t.Desc)
	}
	_ = w.Flush()
}

func printStatusCompact(_ *config.Config, tr *tracker.Tracker) {
	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		t := tr.Tickets[id]
		fmt.Fprintf(color.Output, "%s\t%s\tp%d\t%s\n", id, t.Status, t.Phase, t.Desc)
	}
}

func colorStatus(status string) string {
	switch status {
	case "done":
		return color.New(color.FgGreen).Sprint(status)
	case "running":
		return color.New(color.FgYellow).Sprint(status)
	case "failed":
		return color.New(color.FgRed).Sprint(status)
	case "blocked":
		return color.New(color.FgMagenta).Sprint(status)
	default:
		return status
	}
}

func runLiveStatus(cfgFile string, cfg *config.Config) error {
	trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
	order := normalizedStatusOrder(cfg.PostBuild.Order)
	for {
		tr, err := loadTrackerWithFallback(cfg, trackerPath)
		if err != nil {
			return err
		}

		// Clear screen
		fmt.Fprint(os.Stdout, "[H[2J")

		// Header
		now := time.Now().Format("15:04:05")
		fmt.Fprintf(os.Stdout, "agent-swarm — %s — %s (refresh 1s, q to quit)\n\n", color.New(color.Bold).Sprint(tr.Project), now)

		// Stats bar
		stats := tr.Stats()
		featureStats := statusFeatureStats(tr, order)
		runProgress := statusRunProgressSummary(tr, order)
		fmt.Fprintf(os.Stdout, "Project: %s  |  Phase: %d  |  ",
			color.New(color.Bold).Sprint(tr.Project), tr.ActivePhase())
		fmt.Fprintf(os.Stdout, "%s %s %s %s  |  Total: %d\n\n",
			color.New(color.FgGreen).Sprintf("✅%d", stats.Done),
			color.New(color.FgYellow).Sprintf("🔄%d", stats.Running),
			color.New(color.FgWhite).Sprintf("📋%d", stats.Todo),
			color.New(color.FgRed).Sprintf("❌%d", stats.Failed),
			stats.Total)
		fmt.Fprintf(os.Stdout, "Feature stats: ✅%d 🔄%d 📋%d ❌%d | total=%d\n",
			featureStats.Done,
			featureStats.Running,
			featureStats.Todo,
			featureStats.Failed,
			featureStats.Total,
		)
		fmt.Fprintf(os.Stdout, "Run progress: integration=%s post-build=%d/%d (running=%d failed=%d pending=%d skipped=%d)\n",
			runProgress.IntegrationStatus,
			runProgress.PostBuildDone,
			runProgress.PostBuildTotal,
			runProgress.PostBuildRunning,
			runProgress.PostBuildFailed,
			runProgress.PostBuildPending,
			runProgress.PostBuildSkipped,
		)
		if len(order) > 0 {
			fmt.Fprintf(os.Stdout, "Post-build: %s\n\n", statusPostBuildStepLine(order, runProgress.PostBuildSteps))
		}

		// Progress bar
		if stats.Total > 0 {
			pct := float64(stats.Done) / float64(stats.Total) * 100
			barWidth := 40
			filled := int(float64(barWidth) * float64(stats.Done) / float64(stats.Total))
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
			fmt.Fprintf(os.Stdout, "  [%s] %.0f%%\n\n", color.New(color.FgGreen).Sprint(bar), pct)
		}

		// Table — use fixed-width printf to avoid color code misalignment
		fmt.Fprintf(os.Stdout, "%-8s %5s  %-10s %-18s %s\n", "ID", "PHASE", "STATUS", "DEPENDS", "DESC")
		fmt.Fprintf(os.Stdout, "%-8s %5s  %-10s %-18s %s\n", "──", "─────", "──────", "───────", "────")

		ids := make([]string, 0, len(tr.Tickets))
		for id := range tr.Tickets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			t := tr.Tickets[id]
			rawStatus := t.Status
			coloredStatus := colorStatus(rawStatus)
			pad := 10 - len(rawStatus)
			if pad < 0 {
				pad = 0
			}
			paddedStatus := coloredStatus + strings.Repeat(" ", pad)
			deps := strings.Join(t.Depends, ",")
			if deps == "" {
				deps = "-"
			}
			if len(deps) > 18 {
				deps = deps[:15] + "..."
			}
			desc := t.Desc
			if len(desc) > 35 {
				desc = desc[:32] + "..."
			}
			fmt.Fprintf(os.Stdout, "%-8s %5d  %s %-18s %s\n", id, t.Phase, paddedStatus, deps, desc)
		}

		time.Sleep(1 * time.Second)
	}
}

type statusRunProgress struct {
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

func statusFeatureStats(tr *tracker.Tracker, order []string) tracker.Stats {
	var stats tracker.Stats
	if tr == nil {
		return stats
	}
	stepSet := statusOrderSet(order)
	for id, tk := range tr.Tickets {
		if _, ok := statusPostBuildStepForTicket(id, tk, stepSet); ok {
			continue
		}
		if statusIsIntegrationTicket(id, tk, stepSet) {
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

func statusRunProgressSummary(tr *tracker.Tracker, order []string) statusRunProgress {
	normalizedOrder := normalizedStatusOrder(order)
	out := statusRunProgress{
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

	stepSet := statusOrderSet(normalizedOrder)
	runID := statusChooseRunID(tr, stepSet)
	out.RunID = runID

	seen := map[string]bool{}
	anyStep := false
	for id, tk := range tr.Tickets {
		ticketRunID := strings.TrimSpace(tk.RunID)
		if runID != "" && ticketRunID != "" && ticketRunID != runID {
			continue
		}
		if statusIsIntegrationTicket(id, tk, stepSet) {
			out.IntegrationStatus = statusMergeState(out.IntegrationStatus, statusTicketToState(tk.Status))
			continue
		}
		step, ok := statusPostBuildStepForTicket(id, tk, stepSet)
		if !ok {
			continue
		}
		seen[step] = true
		anyStep = true
		out.PostBuildSteps[step] = statusMergeState(out.PostBuildSteps[step], statusTicketToState(tk.Status))
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

func statusChooseRunID(tr *tracker.Tracker, stepSet map[string]struct{}) string {
	bestID := ""
	bestScore := -1
	for id, tk := range tr.Tickets {
		runID := strings.TrimSpace(tk.RunID)
		if runID == "" {
			continue
		}
		score := 0
		if statusIsIntegrationTicket(id, tk, stepSet) {
			score += 100
		}
		if _, ok := statusPostBuildStepForTicket(id, tk, stepSet); ok {
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

func normalizedStatusOrder(order []string) []string {
	out := make([]string, 0, len(order))
	seen := map[string]struct{}{}
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

func statusOrderSet(order []string) map[string]struct{} {
	set := make(map[string]struct{}, len(order))
	for _, step := range order {
		set[step] = struct{}{}
	}
	return set
}

func statusIsIntegrationTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) bool {
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

func statusPostBuildStepForTicket(id string, tk tracker.Ticket, stepSet map[string]struct{}) (string, bool) {
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

func statusTicketToState(status string) string {
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

func statusMergeState(current, next string) string {
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

func statusPostBuildStepLine(order []string, steps map[string]string) string {
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

func resolveFromConfig(configPath, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(configPath), target)
}
