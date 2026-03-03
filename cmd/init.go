package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <project>",
	Short: "Scaffold project swarm files with standard assets",
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
	if absRoot, err := filepath.Abs(root); err == nil {
		projectName = filepath.Base(absRoot)
	}

	// Create swarm directories
	dirs := []string{
		filepath.Join(root, "swarm", "prompts"),
		filepath.Join(root, "swarm", "features"),
		filepath.Join(root, "swarm", "logs"),
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".agents", "profiles"),
		filepath.Join(root, ".codex", "rules"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	archived, err := archiveLegacyWorkflowFiles(root)
	if err != nil {
		return fmt.Errorf("archive legacy workflow files: %w", err)
	}

	// Write swarm.toml
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

	// Write empty tracker
	tr := &tracker.Tracker{
		Project: projectName,
		Tickets: map[string]tracker.Ticket{},
	}
	trackerPath := filepath.Join(root, cfg.Project.Tracker)
	if err := tr.SaveTo(trackerPath); err != nil {
		return err
	}

	// Copy embedded assets
	copied := 0

	// AGENTS.md
	n, err := copyEmbedDir(assets, "assets/AGENTS.md", root)
	if err != nil {
		return fmt.Errorf("copy AGENTS.md: %w", err)
	}
	copied += n

	// Skills
	n, err = copyEmbedTree(assets, "assets/skills", filepath.Join(root, ".agents", "skills"))
	if err != nil {
		return fmt.Errorf("copy skills: %w", err)
	}
	copied += n

	// Profiles
	n, err = copyEmbedTree(assets, "assets/profiles", filepath.Join(root, ".agents", "profiles"))
	if err != nil {
		return fmt.Errorf("copy profiles: %w", err)
	}
	copied += n

	// Rules
	n, err = copyEmbedTree(assets, "assets/rules", filepath.Join(root, ".codex", "rules"))
	if err != nil {
		return fmt.Errorf("copy rules: %w", err)
	}
	copied += n

	fmt.Printf("✅ Initialized %s\n", projectName)
	fmt.Printf("   swarm.toml + tracker.json\n")
	fmt.Printf("   %d asset files (AGENTS.md, skills, profiles, rules)\n", copied)
	fmt.Printf("   swarm/features/ — feature lifecycle directory\n")
	if len(archived) > 0 {
		fmt.Printf("   archived legacy workflow files: %s\n", strings.Join(archived, ", "))
	}
	return nil
}

func archiveLegacyWorkflowFiles(root string) ([]string, error) {
	legacyFiles := []string{"WORKFLOW_AUTO.md", "sprint.json"}
	existing := make([]string, 0, len(legacyFiles))
	for _, name := range legacyFiles {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, name)
			continue
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
	}
	if len(existing) == 0 {
		return nil, nil
	}

	archiveDir := filepath.Join(root, "swarm", "archive", "legacy-workflow")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive dir %s: %w", archiveDir, err)
	}

	for _, name := range existing {
		src := filepath.Join(root, name)
		dst := filepath.Join(archiveDir, name)
		if _, err := os.Stat(dst); err == nil {
			return nil, fmt.Errorf("archive destination already exists: %s", dst)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %s: %w", dst, err)
		}
		if err := os.Rename(src, dst); err != nil {
			return nil, fmt.Errorf("move %s to %s: %w", src, dst, err)
		}
	}

	sort.Strings(existing)
	return existing, nil
}

// copyEmbedDir copies a single embedded file to destDir preserving filename.
func copyEmbedDir(fsys embed.FS, src string, destDir string) (int, error) {
	data, err := fsys.ReadFile(src)
	if err != nil {
		return 0, err
	}
	name := filepath.Base(src)
	dest := filepath.Join(destDir, name)
	if _, err := os.Stat(dest); err == nil {
		return 0, nil // skip existing
	}
	return 1, os.WriteFile(dest, data, 0o644)
}

// copyEmbedTree copies an embedded directory tree to destDir.
func copyEmbedTree(fsys embed.FS, srcDir string, destDir string) (int, error) {
	count := 0
	err := fs.WalkDir(fsys, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		target := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		// Skip if exists
		if _, err := os.Stat(target); err == nil {
			return nil
		}

		data, err := fsys.ReadFile(path)
		if err != nil {
			return err
		}
		count++
		return os.WriteFile(target, data, 0o644)
	})
	return count, err
}
