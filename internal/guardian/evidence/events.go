package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type GuardianEvent struct {
	Timestamp        string `json:"timestamp"`
	EnforcementPoint string `json:"enforcement_point"`
	RuleID           string `json:"rule_id,omitempty"`
	Result           string `json:"result"`
	Reason           string `json:"reason,omitempty"`
	Target           string `json:"target,omitempty"`
	EvidencePath     string `json:"evidence_path,omitempty"`
}

func AppendGuardianEvent(baseDir string, ev GuardianEvent) error {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	path := filepath.Join(baseDir, "guardian-events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
