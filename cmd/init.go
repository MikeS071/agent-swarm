package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

var initSkipPrereqChecks bool

var initCmd = &cobra.Command{
	Use:   "init <project>",
	Short: "Scaffold project swarm files with standard assets",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project := args[0]
		if err := scaffoldProject(project); err != nil {
			return err
		}
		if initSkipPrereqChecks {
			return nil
		}
		return ensureProjectPrerequisites(cmd.Context(), project)
	},
}

func init() {
	initCmd.Flags().BoolVar(&initSkipPrereqChecks, "skip-prereq-checks", false, "skip post-init compliance checks and watchdog install")
	rootCmd.AddCommand(initCmd)
}

func ensureProjectPrerequisites(ctx context.Context, project string) error {
	root := project
	cfgPath := filepath.Join(root, "swarm.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		return fmt.Errorf("post-init check: missing config %s", cfgPath)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("post-init check: load config: %w", err)
	}
	trackerPath := resolveFromConfig(cfgPath, cfg.Project.Tracker)
	cfg.Project.Tracker = trackerPath
	promptDir := resolveFromConfig(cfgPath, cfg.Project.PromptDir)
	cfg.Project.PromptDir = promptDir

	tr, err := loadTrackerWithFallback(cfg, trackerPath)
	if err != nil {
		return fmt.Errorf("post-init check: load tracker: %w", err)
	}
	if issues := runPrepChecks(cfg, tr, promptDir); len(issues) > 0 {
		return fmt.Errorf("post-init check: prep failed with %d issue(s)", len(issues))
	}
	if err := runWatchWithConfigPath(ctx, cfgPath, "", true, true); err != nil {
		return fmt.Errorf("post-init smoke pass failed: %w", err)
	}
	if err := ensureWatchdogInstalledForConfig(cfgPath); err != nil {
		return fmt.Errorf("post-init watchdog install failed: %w", err)
	}
	return nil
}

func defaultStateDir(projectName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "agent-swarm", "projects", projectName), nil
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
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	cfg.Project.Repo = absRoot
	stateDir, err := defaultStateDir(projectName)
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}
	cfg.Project.StateDir = stateDir
	cfg.Project.Tracker = filepath.Join(stateDir, "tracker.json")
	if base := detectBaseBranch(root); strings.TrimSpace(base) != "" {
		cfg.Project.BaseBranch = base
	}
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

	// Write empty runtime tracker to state dir
	tr := &tracker.Tracker{
		Project: projectName,
		Tickets: map[string]tracker.Ticket{},
	}
	trackerPath := cfg.Project.Tracker
	if err := tr.SaveTo(trackerPath); err != nil {
		return err
	}
	// Write immutable seed tracker in repo for reproducible bootstrap
	seedPath := filepath.Join(root, "swarm", "tracker.seed.json")
	if err := tr.SaveTo(seedPath); err != nil {
		return fmt.Errorf("write tracker seed: %w", err)
	}
	// Ensure runtime events file exists in state dir
	eventsPath := filepath.Join(cfg.Project.StateDir, "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		if err := os.WriteFile(eventsPath, []byte{}, 0o644); err != nil {
			return fmt.Errorf("write events log: %w", err)
		}
	}

	// Copy embedded assets
	copied := 0

	// AGENTS.md
	n, err := copyEmbedDir(assets, "assets/AGENTS.md", root)
	if err != nil {
		return fmt.Errorf("copy AGENTS.md: %w", err)
	}
	copied += n

	// Lifecycle policy
	n, err = copyEmbedDir(assets, "assets/lifecycle-policy.toml", filepath.Join(root, ".agents"))
	if err != nil {
		return fmt.Errorf("copy lifecycle-policy.toml: %w", err)
	}
	copied += n

	// Guardian flow scaffold
	n, err = copyEmbedDir(assets, "assets/flow.v2.yaml", filepath.Join(root, "swarm"))
	if err != nil {
		return fmt.Errorf("copy flow.v2.yaml: %w", err)
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

	if err := ensureGitBootstrap(root, cfg.Project.BaseBranch); err != nil {
		return fmt.Errorf("git bootstrap: %w", err)
	}

	if err := registerProjectInRegistry(projectName, root, cfg.Project.Tracker, cfg.Project.PromptDir); err != nil {
		return fmt.Errorf("register project: %w", err)
	}

	fmt.Printf("✅ Initialized %s\n", projectName)
	fmt.Printf("   swarm.toml + state tracker (%s)\n", cfg.Project.Tracker)
	fmt.Printf("   %d asset files (AGENTS.md, guardian flow, skills, profiles, rules)\n", copied)
	fmt.Printf("   swarm/features/ — feature lifecycle directory\n")
	fmt.Printf("   swarm/tracker.seed.json — immutable seed tracker\n")
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

type projectRegistryEntry struct {
	Description string `json:"description,omitempty"`
	Repo        string `json:"repo"`
	Tracker     string `json:"tracker,omitempty"`
	PromptDir   string `json:"promptDir,omitempty"`
	SpawnScript string `json:"spawnScript,omitempty"`
}

func projectsRegistryPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("OPENCLAW_PROJECTS_REGISTRY")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openclaw", "workspace", "swarm", "projects.json"), nil
}

func registerProjectInRegistry(projectName, root, trackerPath, promptDir string) error {
	registryPath, err := projectsRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return fmt.Errorf("mkdir registry dir: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	absPromptDir := promptDir
	if !filepath.IsAbs(absPromptDir) {
		absPromptDir = filepath.Join(absRoot, promptDir)
	}

	registry := map[string]projectRegistryEntry{}
	if b, err := os.ReadFile(registryPath); err == nil {
		if len(strings.TrimSpace(string(b))) > 0 {
			if err := json.Unmarshal(b, &registry); err != nil {
				return fmt.Errorf("parse registry %s: %w", registryPath, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read registry %s: %w", registryPath, err)
	}

	registry[projectName] = projectRegistryEntry{
		Description: fmt.Sprintf("%s project", projectName),
		Repo:        absRoot,
		Tracker:     trackerPath,
		PromptDir:   absPromptDir,
	}

	out, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(registryPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("write registry %s: %w", registryPath, err)
	}
	return nil
}

func detectBaseBranch(root string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", root, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ensureGitBootstrap(root, baseBranch string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}

	if err := runGit(root, "rev-parse", "--is-inside-work-tree"); err != nil {
		if err := runGit(root, "init", "-b", baseBranch); err != nil {
			if err2 := runGit(root, "init"); err2 != nil {
				return fmt.Errorf("git init: %w", err)
			}
			_ = runGit(root, "branch", "-M", baseBranch)
		}
	}

	if err := runGit(root, "rev-parse", "--verify", "HEAD"); err != nil {
		if err := runGit(root, "add", "-A"); err != nil {
			return fmt.Errorf("git add: %w", err)
		}
		if err := runGit(root, "commit", "-m", "chore: initialize agent-swarm project scaffold"); err != nil {
			_ = runGit(root, "config", "user.name", "agent-swarm")
			_ = runGit(root, "config", "user.email", "agent-swarm@local")
			if err2 := runGit(root, "commit", "-m", "chore: initialize agent-swarm project scaffold"); err2 != nil {
				return fmt.Errorf("git commit initial scaffold: %w", err2)
			}
		}
	}

	if err := runGit(root, "rev-parse", "--verify", "refs/heads/"+baseBranch); err != nil {
		if err := runGit(root, "branch", "-M", baseBranch); err != nil {
			return fmt.Errorf("ensure base branch %s: %w", baseBranch, err)
		}
	}
	return nil
}

func runGit(root string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w (%s)", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}
