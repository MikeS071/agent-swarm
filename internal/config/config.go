package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Project       ProjectConfig       `toml:"project"`
	Backend       BackendConfig       `toml:"backend"`
	Notifications NotificationsConfig `toml:"notifications"`
	Watchdog      WatchdogConfig      `toml:"watchdog"`
	Integration   IntegrationConfig   `toml:"integration"`
	Serve         ServeConfig         `toml:"serve"`
	Install       InstallConfig       `toml:"install"`
	Profiles      ProfilesConfig      `toml:"profiles"`
	Guardian      GuardianConfig      `toml:"guardian"`
	PostBuild     PostBuildConfig     `toml:"post_build"`
	StatusReport  StatusReportConfig  `toml:"status_report"`
	Lifecycle     LifecycleConfig     `toml:"lifecycle"`
}

type ProjectConfig struct {
	Name                string `toml:"name"`
	Repo                string `toml:"repo"`
	StateDir            string `toml:"state_dir"`
	BaseBranch          string `toml:"base_branch"`
	MaxAgents           int    `toml:"max_agents"`
	MinRAMMB            int    `toml:"min_ram_mb"`
	PromptDir           string `toml:"prompt_dir"`
	Tracker             string `toml:"tracker"`
	AutoApprove         bool   `toml:"auto_approve"`
	SpecFile            string `toml:"spec_file"`
	FeaturesDir         string `toml:"features_dir"`
	RequireExplicitRole bool   `toml:"require_explicit_role"`
	RequireVerifyCmd    bool   `toml:"require_verify_cmd"`
}

type BackendConfig struct {
	Type          string `toml:"type"`
	Model         string `toml:"model"`
	Binary        string `toml:"binary"`
	Effort        string `toml:"effort"`
	BypassSandbox bool   `toml:"bypass_sandbox"`
}

type NotificationsConfig struct {
	Type             string `toml:"type"`
	TelegramChatID   string `toml:"telegram_chat_id"`
	TelegramToken    string `toml:"telegram_token"`
	TelegramTokenCmd string `toml:"telegram_token_cmd"`
}

type WatchdogConfig struct {
	Interval     string `toml:"interval"`
	MaxRuntime   string `toml:"max_runtime"`
	StaleTimeout string `toml:"stale_timeout"`
	MaxRetries   int    `toml:"max_retries"`
}

type PostBuildConfig struct {
	Order                 []string   `toml:"order"`
	ParallelGroups        [][]string `toml:"parallel_groups"`
	RequireIntegratedBase bool       `toml:"require_integrated_base"`
	IntegratedBaseBranch  string     `toml:"integrated_base_branch"`
}

type StatusReportConfig struct {
	Enabled          bool   `toml:"enabled"`
	Interval         string `toml:"interval"`
	OnlyWhenRunning  bool   `toml:"only_when_running"`
	SendOnCompletion bool   `toml:"send_on_completion"`
}

type LifecycleConfig struct {
	PolicyFile string `toml:"policy_file"`
}

type IntegrationConfig struct {
	VerifyCmd   string `toml:"verify_cmd"`
	AuditTicket string `toml:"audit_ticket"`
}

type GuardianConfig struct {
	Enabled  bool   `toml:"enabled"`
	FlowFile string `toml:"flow_file"`
	Mode     string `toml:"mode"`
}

type ServeConfig struct {
	Port      int      `toml:"port"`
	CORS      []string `toml:"cors"`
	AuthToken string   `toml:"auth_token"`
}

type InstallConfig struct {
	Method   string `toml:"method"`
	Interval string `toml:"interval"`
	RunMode  string `toml:"run_mode"`
}

type ProfilesConfig struct {
	Architect          string `toml:"architect"`
	CodeAgent          string `toml:"code_agent"`
	TDDGuide           string `toml:"tdd_guide"`
	CodeReviewer       string `toml:"code_reviewer"`
	SecurityReviewer   string `toml:"security_reviewer"`
	E2ERunner          string `toml:"e2e_runner"`
	DocUpdater         string `toml:"doc_updater"`
	RefactorCleaner    string `toml:"refactor_cleaner"`
	BuildErrorResolver string `toml:"build_error_resolver"`
}

func Default() *Config {
	return &Config{
		Project: ProjectConfig{
			Name:                "myproject",
			Repo:                ".",
			StateDir:            "",
			BaseBranch:          "main",
			MaxAgents:           7,
			MinRAMMB:            1024,
			PromptDir:           "swarm/prompts",
			Tracker:             "swarm/tracker.json",
			FeaturesDir:         "swarm/features",
			RequireExplicitRole: true,
			RequireVerifyCmd:    true,
		},
		Backend: BackendConfig{
			Type:          "codex-tmux",
			Model:         "gpt-5.3-codex",
			Binary:        "",
			Effort:        "high",
			BypassSandbox: true,
		},
		Notifications: NotificationsConfig{
			Type:           "stdout",
			TelegramChatID: "",
			TelegramToken:  "",
		},
		Watchdog: WatchdogConfig{
			Interval:     "5m",
			MaxRuntime:   "45m",
			StaleTimeout: "10m",
			MaxRetries:   2,
		},
		Integration: IntegrationConfig{
			VerifyCmd:   "",
			AuditTicket: "",
		},
		Serve: ServeConfig{
			Port:      8090,
			CORS:      nil,
			AuthToken: "",
		},
		Install: InstallConfig{
			Method:   "",
			Interval: "5m",
			RunMode:  "timer",
		},
		Profiles: ProfilesConfig{
			Architect:          ".agents/profiles/architect.md",
			CodeAgent:          ".agents/profiles/code-agent.md",
			TDDGuide:           ".agents/profiles/tdd-guide.md",
			CodeReviewer:       ".agents/profiles/code-reviewer.md",
			SecurityReviewer:   ".agents/profiles/security-reviewer.md",
			E2ERunner:          ".agents/profiles/e2e-runner.md",
			DocUpdater:         ".agents/profiles/doc-updater.md",
			RefactorCleaner:    ".agents/profiles/refactor-cleaner.md",
			BuildErrorResolver: ".agents/profiles/build-error-resolver.md",
		},
		PostBuild: PostBuildConfig{
			Order:                 []string{"doc"},
			ParallelGroups:        nil,
			RequireIntegratedBase: true,
			IntegratedBaseBranch:  "dev",
		},
		Guardian: GuardianConfig{
			Enabled:  true,
			FlowFile: "swarm/flow.v2.yaml",
			Mode:     "advisory",
		},
		StatusReport: StatusReportConfig{
			Enabled:          false,
			Interval:         "5m",
			OnlyWhenRunning:  true,
			SendOnCompletion: true,
		},
		Lifecycle: LifecycleConfig{
			PolicyFile: ".agents/lifecycle-policy.toml",
		},
	}
}

func Load(path string) (*Config, error) {
	configDir := filepath.Dir(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var raw struct {
		Project struct {
			Name *string `toml:"name"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if raw.Project.Name == nil || strings.TrimSpace(*raw.Project.Name) == "" {
		return nil, fmt.Errorf("project.name is required")
	}

	cfg := Default()
	if err := toml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if strings.TrimSpace(cfg.Project.FeaturesDir) == "" {
		cfg.Project.FeaturesDir = "swarm/features"
	}
	if strings.TrimSpace(cfg.StatusReport.Interval) == "" {
		cfg.StatusReport.Interval = "5m"
	}

	// Normalize relative paths so config works regardless of current working directory.
	cfg.Project.Repo = resolveRelative(configDir, cfg.Project.Repo)
	if strings.TrimSpace(cfg.Project.Repo) != "" {
		if abs, err := filepath.Abs(cfg.Project.Repo); err == nil {
			cfg.Project.Repo = abs
		}
	}
	cfg.Project.StateDir = resolveRelative(configDir, cfg.Project.StateDir)
	repoBase := cfg.Project.Repo
	if repoBase == "" {
		repoBase = configDir
	}
	cfg.Project.PromptDir = resolveRelative(repoBase, cfg.Project.PromptDir)
	cfg.Project.Tracker = resolveRelative(repoBase, cfg.Project.Tracker)
	cfg.Project.FeaturesDir = resolveRelative(repoBase, cfg.Project.FeaturesDir)
	cfg.Project.SpecFile = resolveRelative(repoBase, cfg.Project.SpecFile)
	cfg.Lifecycle.PolicyFile = resolveRelative(repoBase, cfg.Lifecycle.PolicyFile)
	if strings.TrimSpace(cfg.Guardian.FlowFile) == "" {
		cfg.Guardian.FlowFile = "swarm/flow.v2.yaml"
	}
	cfg.Guardian.FlowFile = resolveRelative(repoBase, cfg.Guardian.FlowFile)
	cfg.Guardian.Mode = strings.ToLower(strings.TrimSpace(cfg.Guardian.Mode))
	if cfg.Guardian.Mode == "" {
		cfg.Guardian.Mode = "advisory"
	}

	// If state_dir is configured and tracker is still the legacy repo-local
	// default path, move tracker to state dir automatically.
	if strings.TrimSpace(cfg.Project.StateDir) != "" {
		legacyDefault := filepath.Clean(filepath.Join(repoBase, "swarm", "tracker.json"))
		if filepath.Clean(cfg.Project.Tracker) == legacyDefault {
			cfg.Project.Tracker = filepath.Join(cfg.Project.StateDir, "tracker.json")
		}
	}

	// Profile paths are relative to project root.
	cfg.Profiles.Architect = resolveRelative(repoBase, cfg.Profiles.Architect)
	cfg.Profiles.CodeAgent = resolveRelative(repoBase, cfg.Profiles.CodeAgent)
	cfg.Profiles.TDDGuide = resolveRelative(repoBase, cfg.Profiles.TDDGuide)
	cfg.Profiles.CodeReviewer = resolveRelative(repoBase, cfg.Profiles.CodeReviewer)
	cfg.Profiles.SecurityReviewer = resolveRelative(repoBase, cfg.Profiles.SecurityReviewer)
	cfg.Profiles.E2ERunner = resolveRelative(repoBase, cfg.Profiles.E2ERunner)
	cfg.Profiles.DocUpdater = resolveRelative(repoBase, cfg.Profiles.DocUpdater)
	cfg.Profiles.RefactorCleaner = resolveRelative(repoBase, cfg.Profiles.RefactorCleaner)
	cfg.Profiles.BuildErrorResolver = resolveRelative(repoBase, cfg.Profiles.BuildErrorResolver)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	resolveSecrets(cfg)
	return cfg, nil
}

func resolveSecrets(cfg *Config) {
	if cfg.Notifications.TelegramToken == "" && cfg.Notifications.TelegramTokenCmd != "" {
		out, err := exec.Command("sh", "-c", cfg.Notifications.TelegramTokenCmd).Output()
		if err == nil {
			cfg.Notifications.TelegramToken = strings.TrimSpace(string(out))
		}
	}
}

func resolveRelative(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	if strings.HasPrefix(value, "~") {
		return value
	}
	if base == "" {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(base, value))
}

func validate(cfg *Config) error {
	if strings.TrimSpace(cfg.Project.Name) == "" {
		return fmt.Errorf("project.name is required")
	}
	if strings.TrimSpace(cfg.Project.Repo) == "" {
		return fmt.Errorf("project.repo is required")
	}
	if strings.TrimSpace(cfg.Project.BaseBranch) == "" {
		return fmt.Errorf("project.base_branch is required")
	}
	if strings.TrimSpace(cfg.Project.PromptDir) == "" {
		return fmt.Errorf("project.prompt_dir is required")
	}
	if strings.TrimSpace(cfg.Project.Tracker) == "" {
		return fmt.Errorf("project.tracker is required")
	}
	if cfg.Project.MaxAgents <= 0 {
		return fmt.Errorf("project.max_agents must be > 0")
	}
	if cfg.Project.MinRAMMB <= 0 {
		return fmt.Errorf("project.min_ram_mb must be > 0")
	}
	if strings.TrimSpace(cfg.Backend.Type) == "" {
		return fmt.Errorf("backend.type is required")
	}
	if strings.TrimSpace(cfg.Notifications.Type) == "" {
		return fmt.Errorf("notifications.type is required")
	}
	if strings.TrimSpace(cfg.Guardian.FlowFile) == "" {
		return fmt.Errorf("guardian.flow_file is required")
	}
	switch cfg.Guardian.Mode {
	case "advisory", "enforce":
		// ok
	default:
		return fmt.Errorf("guardian.mode must be one of: advisory, enforce")
	}
	return nil
}
