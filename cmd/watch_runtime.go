package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/watchdog"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func runWatchWithConfigPath(ctx context.Context, configPath string, intervalOverride string, once bool, dryRun bool) error {
	return runWatchWithOptions(ctx, configPath, intervalOverride, once, dryRun, false, nil)
}

func runWatchWithOptions(
	ctx context.Context,
	configPath string,
	intervalOverride string,
	once bool,
	dryRun bool,
	allowUnprepared bool,
	warnOut io.Writer,
) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(intervalOverride) != "" {
		cfg.Watchdog.Interval = strings.TrimSpace(intervalOverride)
	}

	trackerPath := resolveFromConfig(configPath, cfg.Project.Tracker)
	cfg.Project.Tracker = trackerPath
	promptDir := resolveFromConfig(configPath, cfg.Project.PromptDir)
	cfg.Project.PromptDir = promptDir

	tr, err := loadTrackerWithFallback(cfg, trackerPath)
	if err != nil {
		return err
	}
	prepIssues := runPrepPipeline(cfg, tr, promptDir)
	if len(prepIssues) > 0 {
		if !allowUnprepared {
			return fmt.Errorf("prep gate failed: %d issue(s); run `agent-swarm prep --config %s`", len(prepIssues), configPath)
		}
		if warnOut != nil {
			if _, err := fmt.Fprintf(warnOut, "WARNING: --allow-unprepared set; continuing despite prep failures (%d issue(s))\n", len(prepIssues)); err != nil {
				return err
			}
			for i, issue := range prepIssues {
				if i >= 5 {
					if _, err := fmt.Fprintf(warnOut, "WARNING: ... and %d more prep issue(s)\n", len(prepIssues)-i); err != nil {
						return err
					}
					break
				}
				if _, err := fmt.Fprintf(warnOut, "WARNING: [%s] %s [%s]: %s\n", issue.Step, issue.Ticket, issue.Field, issue.Reason); err != nil {
					return err
				}
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
	wd.SetConfigPath(configPath)
	wd.SetDryRun(dryRun)
	if cfg.Guardian.Enabled {
		wd.SetGuardian(guardian.NewStrictEvaluator())
	}

	if once {
		return wd.RunOnce(ctx)
	}
	return wd.Run(ctx)
}
