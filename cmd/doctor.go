package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "Show hard-gate readiness status before running swarm",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		cfg.Project.Tracker = trackerPath
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)
		cfg.Project.PromptDir = promptDir

		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}

		issues := runPrepChecks(cfg, tr, promptDir)
		status := map[string]any{
			"project": cfg.Project.Name,
			"ok":      len(issues) == 0,
			"gates": map[string]any{
				"require_explicit_role": cfg.Project.RequireExplicitRole,
				"require_verify_cmd":    cfg.Project.RequireVerifyCmd,
				"guardian_enabled":      cfg.Guardian.Enabled,
				"prep_issues":           len(issues),
			},
			"issues": issues,
		}
		if doctorJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(status)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "project: %s\n", cfg.Project.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "require_explicit_role: %t\n", cfg.Project.RequireExplicitRole)
		fmt.Fprintf(cmd.OutOrStdout(), "require_verify_cmd: %t\n", cfg.Project.RequireVerifyCmd)
		fmt.Fprintf(cmd.OutOrStdout(), "guardian_enabled: %t\n", cfg.Guardian.Enabled)
		if len(issues) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "hard-gate status: OK")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "hard-gate status: FAIL (%d issue(s))\n", len(issues))
		for _, iss := range issues {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s [%s]: %s\n", strings.TrimSpace(iss.Ticket), iss.Field, iss.Reason)
		}
		return fmt.Errorf("doctor failed: %d issue(s)", len(issues))
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "output doctor report as JSON")
	rootCmd.AddCommand(doctorCmd)
}
