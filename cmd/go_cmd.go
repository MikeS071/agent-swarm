package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	guardianevidence "github.com/MikeS071/agent-swarm/internal/guardian/evidence"
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
			ev := guardian.NewPolicyEvaluator(cfg)
			phase := d.CurrentPhase()
			dec, err := ev.Evaluate(cmd.Context(), guardian.Request{
				Event: guardian.EventPhaseTransition,
				Phase: phase,
				Context: map[string]any{
					"tickets": tr.Tickets,
				},
			})
			if err != nil {
				return err
			}
			if strings.TrimSpace(dec.EvidencePath) == "" && (dec.Result == guardian.ResultWarn || dec.Result == guardian.ResultBlock) {
				if p, wErr := guardianevidence.WriteDecisionEvidence(filepath.Join(filepath.Dir(resolveFromConfig(cfgFile, cfg.Project.Tracker)), "guardian"), guardianevidence.DecisionEvidence{
					Event:   string(guardian.EventPhaseTransition),
					Phase:   phase,
					Result:  string(dec.Result),
					RuleID:  strings.TrimSpace(dec.RuleID),
					Reason:  strings.TrimSpace(dec.Reason),
					Context: map[string]any{"tickets": tr.Tickets},
				}); wErr == nil {
					dec.EvidencePath = p
				}
			}
			_ = guardianevidence.AppendGuardianEvent(filepath.Join(filepath.Dir(resolveFromConfig(cfgFile, cfg.Project.Tracker)), "guardian"), guardianevidence.GuardianEvent{
				EnforcementPoint: string(guardian.EventPhaseTransition),
				RuleID:           strings.TrimSpace(dec.RuleID),
				Result:           string(dec.Result),
				Reason:           strings.TrimSpace(dec.Reason),
				Target:           strings.TrimSpace(dec.Target),
				EvidencePath:     strings.TrimSpace(dec.EvidencePath),
			})
			if dec.Result == guardian.ResultBlock {
				return fmt.Errorf("guardian blocked phase transition: %s (rule=%s)", dec.Reason, dec.RuleID)
			}
			if dec.Result == guardian.ResultWarn {
				fmt.Fprintf(os.Stdout, "⚠️ Guardian advisory: %s (rule=%s)\n", dec.Reason, dec.RuleID)
			}
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
