package config

import (
	"fmt"
	"os"
	"os/exec"
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
}

type ProjectConfig struct {
	Name       string `toml:"name"`
	Repo       string `toml:"repo"`
	BaseBranch string `toml:"base_branch"`
	MaxAgents  int    `toml:"max_agents"`
	MinRAMMB   int    `toml:"min_ram_mb"`
	PromptDir  string `toml:"prompt_dir"`
	Tracker     string `toml:"tracker"`
	AutoApprove bool   `toml:"auto_approve"`
	SpecFile       string `toml:"spec_file"`
	DefaultProfile string `toml:"default_profile"`
}

type BackendConfig struct {
	Type          string `toml:"type"`
	Model         string `toml:"model"`
	Binary        string `toml:"binary"`
	Effort        string `toml:"effort"`
	BypassSandbox bool   `toml:"bypass_sandbox"`
}

type NotificationsConfig struct {
	Type           string `toml:"type"`
	TelegramChatID string `toml:"telegram_chat_id"`
	TelegramToken    string `toml:"telegram_token"`
	TelegramTokenCmd string `toml:"telegram_token_cmd"`
}

type WatchdogConfig struct {
	Interval     string `toml:"interval"`
	MaxRuntime   string `toml:"max_runtime"`
	StaleTimeout string `toml:"stale_timeout"`
	MaxRetries   int    `toml:"max_retries"`
}

type IntegrationConfig struct {
	VerifyCmd   string `toml:"verify_cmd"`
	AuditTicket string `toml:"audit_ticket"`
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

func Default() *Config {
	return &Config{
		Project: ProjectConfig{
			Name:       "myproject",
			Repo:       ".",
			BaseBranch: "main",
			MaxAgents:  7,
			MinRAMMB:   1024,
			PromptDir:  "swarm/prompts",
			Tracker:    "swarm/tracker.json",
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
	}
}

func Load(path string) (*Config, error) {
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
	return nil
}
