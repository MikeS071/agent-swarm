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
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/tui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusProject string
var statusJSON bool
var statusWatch bool
var statusCompact bool
var statusLive bool

type statusHints struct {
	Signal        string `json:"signal,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	NextAction    string `json:"next_action,omitempty"`
	Spawnable     int    `json:"spawnable_count,omitempty"`
	TrackerPath   string `json:"tracker_path,omitempty"`
	Warning       string `json:"warning,omitempty"`
}

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
		hints := deriveStatusHints(cfg, tr, trackerPath)
		if div, derr := detectTrackerDivergence(cfg, trackerPath, tr); derr != nil {
			hints.Warning = derr.Error()
		} else if div != nil {
			hints.Warning = div.Error()
		}
		if statusProject != "" && statusProject != tr.Project {
			return fmt.Errorf("project %q not found (tracker project is %q)", statusProject, tr.Project)
		}
		// status is read-only by design: do not mutate tracker/runtime state here.
		if statusLive {
			return runLiveStatus(cfgFile, cfg)
		}
		if statusJSON {
			return printStatusJSON(tr, hints)
		}
		if statusCompact {
			printStatusCompact(tr)
			return nil
		}
		printStatusTable(tr, hints)
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

func printStatusJSON(tr *tracker.Tracker, hints statusHints) error {
	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	tickets := make(map[string]tracker.Ticket, len(ids))
	for _, id := range ids {
		tickets[id] = tr.Tickets[id]
	}

	payload := struct {
		Project     string                    `json:"project"`
		ActivePhase int                       `json:"activePhase"`
		Stats       tracker.Stats             `json:"stats"`
		Spawnable   []string                  `json:"spawnable"`
		Hints       statusHints               `json:"hints"`
		Tickets     map[string]tracker.Ticket `json:"tickets"`
	}{
		Project:     tr.Project,
		ActivePhase: tr.ActivePhase(),
		Stats:       tr.Stats(),
		Spawnable:   tr.GetSpawnable(),
		Hints:       hints,
		Tickets:     tickets,
	}
	enc := json.NewEncoder(color.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printStatusTable(tr *tracker.Tracker, hints statusHints) {
	fmt.Fprintf(color.Output, "Project: %s\n", tr.Project)
	fmt.Fprintf(color.Output, "Active phase: %d\n", tr.ActivePhase())
	stats := tr.Stats()
	fmt.Fprintf(color.Output, "Stats: done=%d running=%d todo=%d failed=%d blocked=%d total=%d\n",
		stats.Done, stats.Running, stats.Todo, stats.Failed, stats.Blocked, stats.Total)
	if strings.TrimSpace(hints.Signal) != "" {
		fmt.Fprintf(color.Output, "Signal: %s\n", hints.Signal)
	}
	if strings.TrimSpace(hints.BlockedReason) != "" {
		fmt.Fprintf(color.Output, "Blocked reason: %s\n", hints.BlockedReason)
	}
	if strings.TrimSpace(hints.NextAction) != "" {
		fmt.Fprintf(color.Output, "Next action: %s\n", hints.NextAction)
	}
	if strings.TrimSpace(hints.Warning) != "" {
		fmt.Fprintf(color.Output, "Warning: %s\n", hints.Warning)
	}
	fmt.Fprintln(color.Output)

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

func printStatusCompact(tr *tracker.Tracker) {
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

func deriveStatusHints(cfg *config.Config, tr *tracker.Tracker, trackerPath string) statusHints {
	h := statusHints{TrackerPath: trackerPath}
	if cfg == nil || tr == nil {
		return h
	}
	d := dispatcher.New(cfg, tr)
	sig, spawnable := d.Evaluate()
	h.Signal = string(sig)
	h.Spawnable = len(spawnable)

	s := tr.Stats()
	switch sig {
	case dispatcher.SignalPhaseGate:
		h.BlockedReason = "PHASE_GATE"
		h.NextAction = "run `agent-swarm go` to approve and continue"
	case dispatcher.SignalBlocked:
		switch {
		case s.Failed > 0:
			h.BlockedReason = "FAILED_TICKETS"
			h.NextAction = "fix/respawn failed tickets, then rerun watchdog"
		case s.Running > 0:
			h.BlockedReason = "WAITING_FOR_RUNNING_TICKETS"
			h.NextAction = "wait for running tickets to finish"
		default:
			h.BlockedReason = "WAITING_FOR_DEPENDENCIES"
			h.NextAction = "inspect dependency chain and prompt/prep gates"
		}
	case dispatcher.SignalSpawn:
		if !d.CanSpawnMore() {
			h.BlockedReason = "CAPACITY_OR_RESOURCE_LIMIT"
			h.NextAction = "reduce running agents or increase capacity"
		}
	}
	return h
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
		fmt.Fprintf(os.Stdout, "Project: %s  |  Phase: %d  |  ",
			color.New(color.Bold).Sprint(tr.Project), tr.ActivePhase())
		fmt.Fprintf(os.Stdout, "%s %s %s %s  |  Total: %d\n\n",
			color.New(color.FgGreen).Sprintf("✅%d", stats.Done),
			color.New(color.FgYellow).Sprintf("🔄%d", stats.Running),
			color.New(color.FgWhite).Sprintf("📋%d", stats.Todo),
			color.New(color.FgRed).Sprintf("❌%d", stats.Failed),
			stats.Total)

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

func resolveFromConfig(configPath, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(configPath), target)
}
