package cmd

import (
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/watchdog"
	"github.com/MikeS071/agent-swarm/internal/worktree"
	"github.com/spf13/cobra"
)

var watchInterval string
var watchOnce bool
var watchDryRun bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run watchdog daemon",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		if strings.TrimSpace(watchInterval) != "" {
			cfg.Watchdog.Interval = watchInterval
		}

		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		cfg.Project.Tracker = trackerPath
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)
		cfg.Project.PromptDir = promptDir

		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}
		d := dispatcher.New(cfg, tr)
		wt := worktree.New(cfg.Project.Repo, "", cfg.Project.BaseBranch)

		be, err := buildBackend(cfg)
		if err != nil {
			return err
		}
		n := buildNotifier(cfg)

		wd := watchdog.New(cfg, tr, d, be, wt, n)
		wd.SetDryRun(watchDryRun)

		if watchOnce {
			return wd.RunOnce(cmd.Context())
		}
		return wd.Run(cmd.Context())
	},
}

func init() {
	watchCmd.Flags().StringVar(&watchInterval, "interval", "", "watchdog loop interval override (e.g. 5m)")
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "run a single pass and exit")
	watchCmd.Flags().BoolVar(&watchDryRun, "dry-run", false, "evaluate actions without executing")
	rootCmd.AddCommand(watchCmd)
}


