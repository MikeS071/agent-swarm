package cmd

import (
	"fmt"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"io"
	"os"

	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/spf13/cobra"
)

func NewFailCmd(d *dispatcher.Dispatcher, out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}

	return &cobra.Command{
		Use:   "fail <ticket>",
		Short: "Mark a ticket as failed",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ticket := args[0]
			if err := d.MarkFailed(ticket); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "marked %s failed\n", ticket)
			return err
		},
	}
}

var failCmd = &cobra.Command{
	Use:   "fail <ticket>",
	Short: "Mark a ticket as failed",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}
		d := dispatcher.New(cfg, tr)
		if err := d.MarkFailed(args[0]); err != nil {
			return err
		}
		if err := tr.SaveTo(trackerPath); err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "marked %s failed\n", args[0])
		return err
	},
}

func init() {
	rootCmd.AddCommand(failCmd)
}
