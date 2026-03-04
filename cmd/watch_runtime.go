package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/guardian"
	"github.com/MikeS071/agent-swarm/internal/watchdog"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func runWatchWithConfigPath(ctx context.Context, configPath string, intervalOverride string, once bool, dryRun bool) error {
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
	if issues := runPrepChecks(cfg, tr, promptDir); len(issues) > 0 {
		return fmt.Errorf("prep gate failed: %d issue(s); run `agent-swarm prep --config %s`", len(issues), configPath)
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
