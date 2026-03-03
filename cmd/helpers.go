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
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	return backend.Build(cfg.Backend.Type, backend.BuildOptions{
		Binary:        cfg.Backend.Binary,
		BypassSandbox: cfg.Backend.BypassSandbox,
	})
}

func buildNotifier(cfg *config.Config) notify.Notifier {
	switch strings.TrimSpace(cfg.Notifications.Type) {
	case "telegram":
		return notify.NewTelegramNotifier(cfg.Notifications.TelegramToken, cfg.Notifications.TelegramChatID)
	default:
		return notify.NewStdoutNotifier(os.Stdout)
	}
}
