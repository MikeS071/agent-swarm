package backend

import (
	"context"
	"time"
)

// AgentBackend controls lifecycle of coding agents.
type AgentBackend interface {
	Spawn(ctx context.Context, cfg SpawnConfig) (AgentHandle, error)
	IsAlive(handle AgentHandle) bool
	HasExited(handle AgentHandle) bool
	GetOutput(handle AgentHandle, lines int) (string, error)
	Kill(handle AgentHandle) error
	Name() string
}

// SpawnConfig configures an agent run.
type SpawnConfig struct {
	TicketID            string
	Branch              string
	WorkDir             string
	ProjectDir          string
	PromptFile          string
	Model               string
	Effort              string
	ExtraFlags          []string
	ProjectName         string
	SpawnFile           string
	ExitFile            string
	ContextManifestPath string
}

// AgentHandle identifies a spawned agent session.
type AgentHandle struct {
	SessionName string
	PID         int
	StartedAt   time.Time
}
