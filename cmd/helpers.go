package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/notify"
)

func buildBackend(cfg *config.Config) (backend.AgentBackend, error) {
	switch strings.TrimSpace(cfg.Backend.Type) {
	case "", "codex-tmux":
		return backend.NewCodexBackend(cfg.Backend.Binary, cfg.Backend.BypassSandbox), nil
	default:
		return nil, fmt.Errorf("unsupported backend type %q", cfg.Backend.Type)
	}
}

func buildNotifier(cfg *config.Config) notify.Notifier {
	switch strings.TrimSpace(cfg.Notifications.Type) {
	case "telegram":
		return notify.NewTelegramNotifier()
	default:
		return notify.NewStdoutNotifier(os.Stdout)
	}
}
