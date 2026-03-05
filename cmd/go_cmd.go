package cmd

import (
	"fmt"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
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
		if cfg.Guardian.Enabled {
			nextPhaseTickets := guardian.NextPhaseTickets(tr, d.CurrentPhase())
			unmet := guardian.CollectTransitionUnmetConditions(guardian.TransitionCheckInput{
				Tickets:             nextPhaseTickets,
				RequireExplicitRole: cfg.Project.RequireExplicitRole,
				RequireVerifyCmd:    cfg.Project.RequireVerifyCmd,
				DefaultVerifyCmd:    cfg.Integration.VerifyCmd,
			})
			dec, err := guardian.NewStrictEvaluator().Evaluate(cmd.Context(), guardian.Request{
				Event: guardian.EventPhaseTransition,
				Phase: d.CurrentPhase(),
				Context: map[string]any{
					"transition":       "go",
					"unmet_conditions": unmet,
				},
			})
			if err != nil {
				return fmt.Errorf("guardian transition check failed: %w", err)
			}
			if dec.Result == guardian.ResultBlock {
				return fmt.Errorf("guardian blocked phase transition: %s", guardian.FormatDecisionBlockReason(dec))
			}
		}

		sig, spawnable := d.ApprovePhaseGate()
		if err := tr.SaveTo(resolveFromConfig(cfgFile, cfg.Project.Tracker)); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "✅ Phase gate approved. Signal: %s\n", sig)
		if len(spawnable) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Spawnable tickets: %v\n", spawnable)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(goCmd)
}
