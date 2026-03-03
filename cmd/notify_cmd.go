package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/spf13/cobra"
)

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Notification and alert controls",
}

var notifyResetCompletionCmd = &cobra.Command{
	Use:   "reset-completion",
	Short: "Reset completion notification marker for the current project",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		markerPath := filepath.Join(filepath.Dir(trackerPath), ".completion-notified")
		if err := os.Remove(markerPath); err != nil {
			if os.IsNotExist(err) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "completion marker already clear")
				return err
			}
			return fmt.Errorf("remove completion marker: %w", err)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "completion marker reset: %s\n", markerPath)
		return err
	},
}

func init() {
	notifyCmd.AddCommand(notifyResetCompletionCmd)
	rootCmd.AddCommand(notifyCmd)
}
