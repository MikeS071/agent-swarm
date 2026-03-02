package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var archivePhase int
var archiveDryRun bool
var archiveRestore bool

var archiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive completed tickets out of the active tracker",
	RunE: func(cmd *cobra.Command, args []string) error {
		if archiveRestore && archiveDryRun {
			return fmt.Errorf("--dry-run cannot be used with --restore")
		}
		if archivePhase < 0 {
			return fmt.Errorf("--phase must be >= 0")
		}

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		archivePath := tracker.DefaultArchivePath(trackerPath)

		if archiveRestore {
			restored, err := tracker.RestoreArchivedTickets(trackerPath, archivePath)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(os.Stdout, "Restored %d tickets from archive\n", restored)
			return err
		}

		summary, err := tracker.ArchiveDoneTickets(trackerPath, archivePath, tracker.ArchiveOptions{
			Phase:  archivePhase,
			DryRun: archiveDryRun,
		})
		if err != nil {
			return err
		}

		verb := "Archived"
		if archiveDryRun {
			verb = "Would archive"
		}
		_, err = fmt.Fprintf(os.Stdout, "%s %d tickets%s\n", verb, summary.Total, formatPhaseSummary(summary))
		return err
	},
}

func init() {
	archiveCmd.Flags().IntVar(&archivePhase, "phase", 0, "archive only done tickets from this phase")
	archiveCmd.Flags().BoolVar(&archiveDryRun, "dry-run", false, "show what would be archived")
	archiveCmd.Flags().BoolVar(&archiveRestore, "restore", false, "restore archived tickets back to tracker")
	rootCmd.AddCommand(archiveCmd)
}

func formatPhaseSummary(summary tracker.ArchiveSummary) string {
	if summary.Total == 0 || len(summary.ByPhase) == 0 {
		return ""
	}
	parts := make([]string, 0, len(summary.ByPhase))
	for _, phase := range summary.Phases() {
		parts = append(parts, "phase "+strconv.Itoa(phase)+": "+strconv.Itoa(summary.ByPhase[phase]))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}
