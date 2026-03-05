package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/roles"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var rolesCheckJSON bool

var rolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "Validate role assets and role references",
}

var rolesCheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check role resolution for tracker tickets",
	SilenceUsage: true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}

		report := roles.Check(cfg.Project.Repo, tr)
		if rolesCheckJSON {
			if err := writeRolesReportJSON(cmd, report); err != nil {
				return err
			}
		}

		if report.OK() {
			if !rolesCheckJSON {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "roles ok"); err != nil {
					return fmt.Errorf("write roles result: %w", err)
				}
			}
			return nil
		}

		if !rolesCheckJSON {
			for _, failure := range report.Failures {
				line := formatRoleFailure(failure)
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return fmt.Errorf("write roles failure: %w", err)
				}
			}
		}
		return fmt.Errorf("roles check failed: %d issue(s)", len(report.Failures))
	},
}

func init() {
	rolesCheckCmd.Flags().BoolVar(&rolesCheckJSON, "json", false, "output role check report as JSON")
	rolesCmd.AddCommand(rolesCheckCmd)
	rootCmd.AddCommand(rolesCmd)
}

func writeRolesReportJSON(cmd *cobra.Command, report roles.Report) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"ok":       report.OK(),
		"roles":    report.Roles,
		"failures": report.Failures,
	}); err != nil {
		return fmt.Errorf("encode roles report: %w", err)
	}
	return nil
}

func formatRoleFailure(f roles.Failure) string {
	var b strings.Builder
	b.WriteString(f.Asset)
	if f.Role != "" {
		b.WriteString(" role=")
		b.WriteString(f.Role)
	}
	if len(f.Tickets) > 0 {
		b.WriteString(" tickets=")
		b.WriteString(strings.Join(f.Tickets, ","))
	}
	if f.Path != "" {
		b.WriteString(" [")
		b.WriteString(f.Path)
		b.WriteString("]")
	}
	if f.Reason != "" {
		b.WriteString(": ")
		b.WriteString(f.Reason)
	}
	return b.String()
}
