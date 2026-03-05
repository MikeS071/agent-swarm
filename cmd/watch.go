package cmd

import (
	"github.com/spf13/cobra"
)

var watchInterval string
var watchOnce bool
var watchDryRun bool
var watchAllowUnprepared bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run watchdog daemon",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runWatchWithOptions(cmd.Context(), cfgFile, watchInterval, watchOnce, watchDryRun, watchAllowUnprepared, cmd.ErrOrStderr())
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchInterval, "interval", "", "watchdog loop interval override (e.g. 5m)")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "run a single pass and exit")
	watchCmd.Flags().BoolVar(&watchDryRun, "dry-run", false, "evaluate actions without executing")
	watchCmd.Flags().BoolVar(&watchAllowUnprepared, "allow-unprepared", false, "bypass prep gate and continue with loud warnings")
	rootCmd.AddCommand(watchCmd)
}
