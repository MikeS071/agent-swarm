package watchdog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneEventsFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "events.jsonl")
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	oldEv := Event{Type: "old", Timestamp: now.Add(-31 * 24 * time.Hour)}
	newEv := Event{Type: "new", Timestamp: now.Add(-2 * time.Hour)}
	bo, _ := json.Marshal(oldEv)
	bn, _ := json.Marshal(newEv)
	_ = os.WriteFile(p, append(append(bo, '\n'), append(bn, '\n')...), 0o644)

	if err := pruneEventsFile(p, now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) == "" {
		t.Fatal("expected remaining events")
	}
	if string(b) == string(append(bo, '\n')) {
		t.Fatal("old event not pruned")
	}
}

func TestWriteDailyRollup(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "events.jsonl")
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	ev1 := Event{Type: "ticket_done", Ticket: "a", Timestamp: time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)}
	ev2 := Event{Type: "ticket_done", Ticket: "a", Timestamp: time.Date(2026, 3, 3, 11, 0, 0, 0, time.UTC)}
	ev3 := Event{Type: "respawn", Ticket: "b", Timestamp: time.Date(2026, 3, 2, 11, 0, 0, 0, time.UTC)}
	for _, ev := range []Event{ev1, ev2, ev3} {
		b, _ := json.Marshal(ev)
		f, _ := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
	if err := writeDailyRollup(p, now); err != nil {
		t.Fatal(err)
	}
	r := filepath.Join(d, "rollups", "2026-03-03.json")
	b, err := os.ReadFile(r)
	if err != nil {
		t.Fatal(err)
	}
	var sum telemetrySummary
	if err := json.Unmarshal(b, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.TotalEvents != 2 {
		t.Fatalf("got %d events, want 2", sum.TotalEvents)
	}
	if sum.ByType["ticket_done"] != 2 {
		t.Fatalf("bad by_type: %#v", sum.ByType)
	}
}
