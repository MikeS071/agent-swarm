package cmd

import (
	"fmt"
	"os"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var goCmd = &cobra.Command{
	Use:   "go",
	Short: "Approve current phase gate and advance to next phase",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		tr, err := tracker.Load(resolveFromConfig(cfgFile, cfg.Project.Tracker))
		if err != nil {
			return err
		}

		d := dispatcher.New(cfg, tr)
		sig, _ := d.Evaluate()
		if sig != dispatcher.SignalPhaseGate {
			return fmt.Errorf("no phase gate reached (current signal: %s)", sig)
		}

		sig, spawnable := d.ApprovePhaseGate()
		if err := tr.SaveTo(resolveFromConfig(cfgFile, cfg.Project.Tracker)); err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "✅ Phase gate approved. Signal: %s\n", sig)
		if len(spawnable) > 0 {
			fmt.Fprintf(os.Stdout, "Spawnable tickets: %v\n", spawnable)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(goCmd)
}
