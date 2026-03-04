package cmd

import (
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/spf13/cobra"
)

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Manage ticket prompt templates",
}

var promptsCheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Report todo tickets missing prompt files",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		missing, err := CheckPrompts(trackerPath, promptDir)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "all todo tickets have prompts")
			return err
		}
		for _, ticketID := range missing {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), ticketID); err != nil {
				return err
			}
		}
		return fmt.Errorf("missing prompts: %s", strings.Join(missing, ", "))
	},
}

func init() {
	promptsCmd.AddCommand(promptsCheckCmd)
	rootCmd.AddCommand(promptsCmd)
}
