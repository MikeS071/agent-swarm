package cmd

import (
	"errors"
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

var promptsGenCmd = &cobra.Command{
	Use:          "gen <ticket>",
	Short:        "Generate prompt template for a ticket",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		path, err := GeneratePrompt(trackerPath, promptDir, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	},
}

var (
	promptsBuildAll    bool
	promptsBuildStrict bool
)

var promptsBuildCmd = &cobra.Command{
	Use:          "build <ticket>",
	Short:        "Compile deterministic execution prompt(s)",
	SilenceUsage: true,
	Args:         cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		ticketID := ""
		if promptsBuildAll {
			if len(args) > 0 {
				return errors.New("ticket argument is not allowed with --all")
			}
		} else {
			if len(args) != 1 {
				return errors.New("requires exactly one ticket or --all")
			}
			ticketID = args[0]
		}

		policy := BuildPromptPolicyContext(cfg, cfgFile)
		built, err := BuildPrompts(trackerPath, promptDir, ticketID, promptsBuildAll, promptsBuildStrict, policy)
		if err != nil {
			return err
		}
		for _, artifact := range built {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), artifact.PromptPath); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), artifact.ManifestPath); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	promptsCmd.AddCommand(promptsCheckCmd)
	promptsCmd.AddCommand(promptsGenCmd)
	promptsCmd.AddCommand(promptsBuildCmd)

	promptsBuildCmd.Flags().BoolVar(&promptsBuildAll, "all", false, "build prompts for all tickets")
	promptsBuildCmd.Flags().BoolVar(&promptsBuildStrict, "strict", false, "fail when required structured fields are missing")

	rootCmd.AddCommand(promptsCmd)
}
