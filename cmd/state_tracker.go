package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

// loadTrackerWithFallback loads tracker from configured state path, with
// one-time import fallback from legacy repo-local swarm/tracker.json.
func loadTrackerWithFallback(cfg *config.Config, trackerPath string) (*tracker.Tracker, error) {
	tr, err := tracker.Load(trackerPath)
	if err == nil {
		return tr, nil
	}

	// Fallback import: if configured tracker is missing, import from legacy path once.
	legacy := filepath.Join(cfg.Project.Repo, "swarm", "tracker.json")
	if filepath.Clean(legacy) == filepath.Clean(trackerPath) {
		return nil, err
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		return nil, err
	}

	legacyTracker, lerr := tracker.Load(legacy)
	if lerr != nil {
		return nil, err
	}
	if mkErr := os.MkdirAll(filepath.Dir(trackerPath), 0o755); mkErr != nil {
		return nil, fmt.Errorf("mkdir tracker state dir: %w", mkErr)
	}
	if saveErr := legacyTracker.SaveTo(trackerPath); saveErr != nil {
		return nil, fmt.Errorf("import legacy tracker from %s to %s: %w", legacy, trackerPath, saveErr)
	}

	// Create events file in same state directory if missing.
	eventsPath := filepath.Join(filepath.Dir(trackerPath), "events.jsonl")
	if _, eerr := os.Stat(eventsPath); os.IsNotExist(eerr) {
		_ = os.WriteFile(eventsPath, []byte{}, 0o644)
	}

	if strings.TrimSpace(cfg.Project.StateDir) != "" {
		fmt.Fprintf(os.Stderr, "[agent-swarm] imported legacy tracker into state dir: %s\n", trackerPath)
	}

	return tracker.Load(trackerPath)
}
