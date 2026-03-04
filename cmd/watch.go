package cmd

import (
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/watchdog"
	"github.com/MikeS071/agent-swarm/internal/worktree"
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

		tr, err := loadTrackerWithFallback(cfg, trackerPath)
		if err != nil {
			return err
		}
		prepIssues := runPrepPipeline(cfg, tr, promptDir)
		if len(prepIssues) > 0 {
			if !watchAllowUnprepared {
				return fmt.Errorf("prep gate failed: %d issue(s); run `agent-swarm prep`", len(prepIssues))
			}
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: --allow-unprepared set; continuing despite prep failures (%d issue(s))\n", len(prepIssues)); err != nil {
				return err
			}
			for i, issue := range prepIssues {
				if i >= 5 {
					if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: ... and %d more prep issue(s)\n", len(prepIssues)-i); err != nil {
						return err
					}
					break
				}
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: [%s] %s [%s]: %s\n", issue.Step, issue.Ticket, issue.Field, issue.Reason); err != nil {
					return err
				}
			}
		}
		d := dispatcher.New(cfg, tr)
		wt := worktree.New(cfg.Project.Repo, "", cfg.Project.BaseBranch)

		be, err := buildBackend(cfg)
		if err != nil {
			return err
		}
		n := buildNotifier(cfg)

		wd := watchdog.New(cfg, tr, d, be, wt, n)
		wd.SetConfigPath(cfgFile)
		wd.SetDryRun(watchDryRun)
		if cfg.Guardian.Enabled {
			wd.SetGuardian(guardian.NewStrictEvaluator())
		}

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
	watchCmd.Flags().BoolVar(&watchAllowUnprepared, "allow-unprepared", false, "bypass prep gate and continue with loud warnings")
	rootCmd.AddCommand(watchCmd)
}
