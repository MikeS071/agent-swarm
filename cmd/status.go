package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusProject string
var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tracker status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		tr, err := tracker.Load(resolveFromConfig(cfgFile, cfg.Project.Tracker))
		if err != nil {
			return err
		}
		if statusProject != "" && statusProject != tr.Project {
			return fmt.Errorf("project %q not found (tracker project is %q)", statusProject, tr.Project)
		}
		if statusJSON {
			return printStatusJSON(tr)
		}
		printStatusTable(tr)
		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&statusProject, "project", "", "project name")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output json")
	rootCmd.AddCommand(statusCmd)
}

func printStatusJSON(tr *tracker.Tracker) error {
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
		Tickets     map[string]tracker.Ticket `json:"tickets"`
	}{
		Project:     tr.Project,
		ActivePhase: tr.ActivePhase(),
		Stats:       tr.Stats(),
		Spawnable:   tr.GetSpawnable(),
		Tickets:     tickets,
	}
	enc := json.NewEncoder(color.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printStatusTable(tr *tracker.Tracker) {
	fmt.Fprintf(color.Output, "Project: %s\n", tr.Project)
	fmt.Fprintf(color.Output, "Active phase: %d\n", tr.ActivePhase())
	stats := tr.Stats()
	fmt.Fprintf(color.Output, "Stats: done=%d running=%d todo=%d failed=%d blocked=%d total=%d\n\n",
		stats.Done, stats.Running, stats.Todo, stats.Failed, stats.Blocked, stats.Total)

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

func resolveFromConfig(configPath, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(configPath), target)
}
