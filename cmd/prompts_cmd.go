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

var promptsBuildCmd = &cobra.Command{
	Use:          "build [ticket-id]",
	Short:        "Build deterministic prompt output and manifest(s)",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)
		repoRoot := resolveFromConfig(cfgFile, cfg.Project.Repo)
		specPath := ""
		if strings.TrimSpace(cfg.Project.SpecFile) != "" {
			specPath = resolveFromConfig(cfgFile, cfg.Project.SpecFile)
		}

		buildAll, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}

		built, err := BuildPrompts(trackerPath, promptDir, repoRoot, specPath, buildAll, args)
		if err != nil {
			return err
		}
		for _, ticketID := range built {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "built %s\n", ticketID); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	promptsBuildCmd.Flags().Bool("all", false, "build prompts for all tickets")
	promptsCmd.AddCommand(promptsCheckCmd)
	promptsCmd.AddCommand(promptsBuildCmd)
	rootCmd.AddCommand(promptsCmd)
}
