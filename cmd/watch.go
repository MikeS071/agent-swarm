package cmd

import (
	"github.com/spf13/cobra"
)

var watchInterval string
var watchOnce bool
var watchDryRun bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run watchdog daemon",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runWatchWithConfigPath(cmd.Context(), cfgFile, watchInterval, watchOnce, watchDryRun)
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchInterval, "interval", "", "watchdog loop interval override (e.g. 5m)")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "run a single pass and exit")
	watchCmd.Flags().BoolVar(&watchDryRun, "dry-run", false, "evaluate actions without executing")
	rootCmd.AddCommand(watchCmd)
}
