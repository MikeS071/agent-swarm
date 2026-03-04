package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MikeS071/agent-swarm/internal/guardian/engine"
)

// Event is an append-only guardian decision record.
type Event struct {
	Result   engine.Result `json:"result"`
	Rule     string        `json:"rule"`
	Reason   string        `json:"reason,omitempty"`
	Target   string        `json:"target,omitempty"`
	Evidence string        `json:"evidence,omitempty"`
	Time     time.Time     `json:"time"`
}

type EventWriter struct {
	path string
	mu   sync.Mutex
}

func NewEventWriter(path string) *EventWriter {
	return &EventWriter{path: path}
}

// AppendDecision writes a single guardian decision as JSONL.
func (w *EventWriter) AppendDecision(d engine.Decision) error {
	if w == nil || strings.TrimSpace(w.path) == "" {
		return nil
	}

	ev := Event{
		Result:   d.Result,
		Rule:     d.Rule,
		Reason:   d.Reason,
		Target:   d.Target,
		Evidence: d.Evidence,
		Time:     d.Time.UTC(),
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}

	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal guardian event: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return fmt.Errorf("mkdir guardian event dir: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open guardian events file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write guardian event: %w", err)
	}
	return nil
}
