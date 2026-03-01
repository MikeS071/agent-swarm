package cmd

import (
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var addTicketDeps string
var addTicketPhase int
var addTicketDesc string

var addTicketCmd = &cobra.Command{
	Use:   "add-ticket <id>",
	Short: "Add a ticket to tracker",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if id == "" {
			return fmt.Errorf("ticket id cannot be empty")
		}
		if addTicketPhase <= 0 {
			return fmt.Errorf("--phase must be > 0")
		}

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}
		if _, exists := tr.Tickets[id]; exists {
			return fmt.Errorf("ticket %q already exists", id)
		}

		deps := parseDeps(addTicketDeps)
		tr.Tickets[id] = tracker.Ticket{
			Status:  "todo",
			Phase:   addTicketPhase,
			Depends: deps,
			Branch:  "feat/" + id,
			Desc:    addTicketDesc,
		}
		return tr.SaveTo(trackerPath)
	},
}

func init() {
	addTicketCmd.Flags().StringVar(&addTicketDeps, "deps", "", "comma-separated dependency ticket ids")
	addTicketCmd.Flags().IntVar(&addTicketPhase, "phase", 1, "ticket phase")
	addTicketCmd.Flags().StringVar(&addTicketDesc, "desc", "", "ticket description")
	rootCmd.AddCommand(addTicketCmd)
}

func parseDeps(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	deps := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deps = append(deps, id)
	}
	return deps
}
