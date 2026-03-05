package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DecisionEvidence struct {
	Timestamp string         `json:"timestamp"`
	Event     string         `json:"event"`
	TicketID  string         `json:"ticket_id,omitempty"`
	Phase     int            `json:"phase,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Result    string         `json:"result"`
	RuleID    string         `json:"rule_id,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
}

var safeNameRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func WriteDecisionEvidence(baseDir string, ev DecisionEvidence) (string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return "", nil
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	dir := filepath.Join(baseDir, "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	ticket := sanitizePart(ev.TicketID)
	if ticket == "" {
		ticket = "run"
	}
	rule := sanitizePart(ev.RuleID)
	if rule == "" {
		rule = "guardian"
	}
	eventPart := sanitizePart(ev.Event)
	if eventPart == "" {
		eventPart = "event"
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("%s-%s-%s-%s.json", ts, eventPart, ticket, rule)
	path := filepath.Join(dir, name)

	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func sanitizePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = safeNameRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return ""
	}
	return s
}
