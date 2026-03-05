package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

// loadTrackerWithFallback loads tracker from configured state path, with
// one-time import fallback from legacy repo-local swarm/tracker.json.
type trackerDivergence struct {
	ActivePath    string
	LegacyPath    string
	ActiveTickets int
	LegacyTickets int
}

func (d *trackerDivergence) Error() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("tracker divergence detected: active=%s (%d tickets) vs legacy=%s (%d tickets)",
		d.ActivePath, d.ActiveTickets, d.LegacyPath, d.LegacyTickets)
}

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

func detectTrackerDivergence(cfg *config.Config, trackerPath string, active *tracker.Tracker) (*trackerDivergence, error) {
	if cfg == nil {
		return nil, nil
	}
	if strings.TrimSpace(cfg.Project.StateDir) == "" {
		return nil, nil
	}
	legacyPath := filepath.Join(cfg.Project.Repo, "swarm", "tracker.json")
	if filepath.Clean(legacyPath) == filepath.Clean(trackerPath) {
		return nil, nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return nil, nil
	}
	legacy, err := tracker.Load(legacyPath)
	if err != nil {
		// Legacy tracker may be stale/invalid; ignore as it is not active source.
		return nil, nil
	}
	if len(legacy.Tickets) == 0 {
		return nil, nil
	}
	if active == nil {
		active, err = tracker.Load(trackerPath)
		if err != nil {
			return nil, err
		}
	}
	if len(active.Tickets) == 0 {
		return nil, nil
	}
	if trackersEquivalent(active, legacy) {
		return nil, nil
	}
	return &trackerDivergence{
		ActivePath:    trackerPath,
		LegacyPath:    legacyPath,
		ActiveTickets: len(active.Tickets),
		LegacyTickets: len(legacy.Tickets),
	}, nil
}

func trackersEquivalent(a, b *tracker.Tracker) bool {
	if a == nil || b == nil {
		return a == b
	}
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}
