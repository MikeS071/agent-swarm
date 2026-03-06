package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/guardian/schema"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var guardianCheckEvent string
var guardianCheckTicket string
var guardianCheckPhase int
var guardianCheckJSON bool

var guardianValidateCmd = &cobra.Command{
	Use:          "validate",
	Short:        "Validate guardian flow policy file",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		policy, err := schema.Load(cfg.Guardian.FlowFile)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "guardian policy valid: version=%d mode=%s rules=%d\n", policy.Version, policy.Mode, len(policy.Rules))
		return nil
	},
}

var guardianCheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Evaluate guardian policy for a selected enforcement point",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		tr, err := tracker.Load(resolveFromConfig(cfgFile, cfg.Project.Tracker))
		if err != nil {
			return err
		}

		ev, err := parseGuardianEvent(guardianCheckEvent)
		if err != nil {
			return err
		}

		req := guardian.Request{
			Event:    ev,
			TicketID: strings.TrimSpace(guardianCheckTicket),
			Phase:    guardianCheckPhase,
			Context: map[string]any{
				"tickets": tr.Tickets,
			},
		}
		if strings.TrimSpace(req.TicketID) != "" {
			tk, ok := tr.Tickets[req.TicketID]
			if !ok {
				return fmt.Errorf("ticket not found: %s", req.TicketID)
			}
			req.Phase = tk.Phase
			req.Context["desc"] = tk.Desc
			req.Context["verify_cmd"] = tk.VerifyCmd
			req.Context["profile"] = tk.Profile
		}
		if req.Phase == 0 {
			req.Phase = tr.ActivePhase()
		}

		dec, err := guardian.NewPolicyEvaluator(cfg).Evaluate(cmd.Context(), req)
		if err != nil {
			return err
		}
		if guardianCheckJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(dec)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "guardian check: result=%s rule=%s reason=%s target=%s\n", dec.Result, dec.RuleID, dec.Reason, dec.Target)
		return nil
	},
}

func parseGuardianEvent(v string) (guardian.Event, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "before_spawn":
		return guardian.EventBeforeSpawn, nil
	case "before_mark_done":
		return guardian.EventBeforeMarkDone, nil
	case "transition":
		return guardian.EventPhaseTransition, nil
	case "post_build_complete":
		return guardian.EventPostBuildDone, nil
	default:
		return "", fmt.Errorf("invalid guardian event %q (expected before_spawn|before_mark_done|transition|post_build_complete)", v)
	}
}
