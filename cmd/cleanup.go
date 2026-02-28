package cmd

import (
	"time"

	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func CleanupWorktrees(m *worktree.Manager, olderThan time.Duration) ([]string, error) {
	return m.CleanupOlderThan(olderThan)
}
