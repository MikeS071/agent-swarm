package cmd

import (
	"fmt"

	"github.com/MikeS071/agent-swarm/internal/config"
	trackermigrate "github.com/MikeS071/agent-swarm/internal/tracker/migrate"
	"github.com/spf13/cobra"
)

var migratePostBuildRunScopeDryRun bool
var migratePostBuildRunScopeApply bool

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run project migrations",
}

var migratePostBuildRunScopeCmd = &cobra.Command{
	Use:          "post-build-run-scope",
	Short:        "Migrate legacy per-feature post-build graph to run-level state",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if migratePostBuildRunScopeDryRun == migratePostBuildRunScopeApply {
			return fmt.Errorf("exactly one of --dry-run or --apply must be set")
		}

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		opts := trackermigrate.PostBuildRunScopeOptions{Steps: cfg.PostBuild.Order}

		var res trackermigrate.PostBuildRunScopeResult
		if migratePostBuildRunScopeApply {
			res, err = trackermigrate.ApplyPostBuildRunScope(trackerPath, opts)
		} else {
			res, err = trackermigrate.PreviewPostBuildRunScope(trackerPath, opts)
		}
		if err != nil {
			return err
		}

		summary := trackermigrate.FormatPostBuildRunScopeSummary(res, migratePostBuildRunScopeApply)
		_, err = fmt.Fprintln(cmd.OutOrStdout(), summary)
		return err
	},
}

func init() {
	migratePostBuildRunScopeCmd.Flags().BoolVar(&migratePostBuildRunScopeDryRun, "dry-run", false, "show the transformation without writing tracker changes")
	migratePostBuildRunScopeCmd.Flags().BoolVar(&migratePostBuildRunScopeApply, "apply", false, "apply the transformation to tracker")

	migrateCmd.AddCommand(migratePostBuildRunScopeCmd)
	rootCmd.AddCommand(migrateCmd)
}
