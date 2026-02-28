package cmd

import (
	"fmt"
	"time"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/worktree"
	"github.com/spf13/cobra"
)

var cleanupOlderThan string

var cleanupCmd = &cobra.Command{
	Use:          "cleanup",
	Short:        "Remove stale worktrees",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		d, err := time.ParseDuration(cleanupOlderThan)
		if err != nil {
			return fmt.Errorf("invalid --older-than duration: %w", err)
		}

		repoDir := resolveFromConfig(cfgFile, cfg.Project.Repo)
		m := worktree.New(repoDir, "", cfg.Project.BaseBranch)
		removed, err := CleanupWorktrees(m, d)
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "no worktrees removed")
			return err
		}
		for _, ticketID := range removed {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), ticketID); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	cleanupCmd.Flags().StringVar(&cleanupOlderThan, "older-than", "24h", "remove worktrees older than this duration")
	rootCmd.AddCommand(cleanupCmd)
}
