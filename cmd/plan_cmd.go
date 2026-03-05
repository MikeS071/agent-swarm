package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var planOptimizeApply bool
var planOptimizeJSON bool
var planOptimizeOnlyTodo bool

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan utilities (analysis and optimization)",
}

var planOptimizeCmd = &cobra.Command{
	Use:          "optimize",
	Short:        "Optimize ticket priorities for higher parallel throughput",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		cfg.Project.Tracker = trackerPath

		tr, err := loadTrackerWithFallback(cfg, trackerPath)
		if err != nil {
			return err
		}

		changes := tr.OptimizePriorities(tracker.OptimizeOptions{OnlyTodo: planOptimizeOnlyTodo})
		sort.Slice(changes, func(i, j int) bool {
			if changes[i].NewPriority == changes[j].NewPriority {
				return changes[i].TicketID < changes[j].TicketID
			}
			return changes[i].NewPriority > changes[j].NewPriority
		})

		if planOptimizeApply {
			if err := tr.Save(); err != nil {
				return err
			}
		}

		if planOptimizeJSON {
			payload := map[string]any{
				"applied": planOptimizeApply,
				"count":   len(changes),
				"changes": changes,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		}

		if len(changes) == 0 {
			if planOptimizeApply {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no priority changes; tracker unchanged")
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no priority changes (dry-run)")
			}
			return nil
		}

		mode := "dry-run"
		if planOptimizeApply {
			mode = "applied"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "optimization %s: %d ticket(s) updated\n", mode, len(changes))
		for _, ch := range changes {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s: %d -> %d (depth=%d descendants=%d)\n", ch.TicketID, ch.OldPriority, ch.NewPriority, ch.CriticalPath, ch.Descendants)
		}
		if !planOptimizeApply {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "run with --apply to persist tracker priority changes")
		}
		return nil
	},
}

func init() {
	planOptimizeCmd.Flags().BoolVar(&planOptimizeApply, "apply", false, "persist optimized priorities to tracker")
	planOptimizeCmd.Flags().BoolVar(&planOptimizeJSON, "json", false, "output optimization result as JSON")
	planOptimizeCmd.Flags().BoolVar(&planOptimizeOnlyTodo, "only-todo", true, "optimize only todo tickets")
	planCmd.AddCommand(planOptimizeCmd)
	rootCmd.AddCommand(planCmd)
}
