package watchdog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type exitMarker struct {
	TicketID        string `json:"ticket_id"`
	EndedAt         string `json:"ended_at"`
	ProcessExitCode int    `json:"process_exit_code"`
	LogPath         string `json:"log_path"`
	WorkDir         string `json:"work_dir"`
	HeadSHA         string `json:"head_sha"`
}

func (w *Watchdog) runtimeStateRoot() string {
	if w == nil || w.config == nil {
		return ""
	}
	if strings.TrimSpace(w.config.Project.StateDir) != "" {
		return strings.TrimSpace(w.config.Project.StateDir)
	}
	if strings.TrimSpace(w.config.Project.Tracker) != "" {
		return filepath.Dir(strings.TrimSpace(w.config.Project.Tracker))
	}
	return ""
}

func (w *Watchdog) runtimeTicketDir(ticketID string) string {
	root := w.runtimeStateRoot()
	if root == "" || strings.TrimSpace(ticketID) == "" {
		return ""
	}
	return filepath.Join(root, "runs", strings.TrimSpace(ticketID))
}

func (w *Watchdog) ticketSpawnFile(ticketID string) string {
	d := w.runtimeTicketDir(ticketID)
	if d == "" {
		return ""
	}
	return filepath.Join(d, "spawn.json")
}

func (w *Watchdog) ticketExitFile(ticketID string) string {
	d := w.runtimeTicketDir(ticketID)
	if d == "" {
		return ""
	}
	return filepath.Join(d, "exit.json")
}

func (w *Watchdog) guardianEvidenceDir(ticketID string) string {
	d := w.runtimeTicketDir(ticketID)
	if d == "" {
		return ""
	}
	return filepath.Join(d, "guardian")
}

func (w *Watchdog) writeSpawnMarker(ticketID string) {
	d := w.runtimeTicketDir(ticketID)
	if d == "" {
		return
	}
	_ = os.MkdirAll(d, 0o755)
	spawnFile := filepath.Join(d, "spawn.json")
	exitFile := filepath.Join(d, "exit.json")
	_ = os.Remove(exitFile)
	payload := map[string]any{
		"ticket_id":  ticketID,
		"started_at": time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(payload)
	_ = os.WriteFile(spawnFile, b, 0o644)
}

func (w *Watchdog) readExitMarker(ticketID string) (exitMarker, bool) {
	path := w.ticketExitFile(ticketID)
	if strings.TrimSpace(path) == "" {
		return exitMarker{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return exitMarker{}, false
	}
	var m exitMarker
	if err := json.Unmarshal(b, &m); err != nil {
		return exitMarker{}, false
	}
	return m, true
}
