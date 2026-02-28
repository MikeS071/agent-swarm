package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <project>",
	Short: "Scaffold project swarm files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project := args[0]
		return scaffoldProject(project)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func scaffoldProject(project string) error {
	root := project
	projectName := filepath.Base(filepath.Clean(project))
	if err := os.MkdirAll(filepath.Join(root, "swarm", "prompts"), 0o755); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	cfg := config.Default()
	cfg.Project.Name = projectName

	cfgBytes, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cfgPath := filepath.Join(root, "swarm.toml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("%s already exists", cfgPath)
	}
	if err := os.WriteFile(cfgPath, cfgBytes, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	tr := &tracker.Tracker{
		Project: projectName,
		Tickets: map[string]tracker.Ticket{},
	}
	trackerPath := filepath.Join(root, cfg.Project.Tracker)
	if err := tr.Save(trackerPath); err != nil {
		return err
	}

	return nil
}
