package watchdog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type telemetrySummary struct {
	DateUTC     string         `json:"date_utc"`
	GeneratedAt string         `json:"generated_at"`
	TotalEvents int            `json:"total_events"`
	ByType      map[string]int `json:"by_type"`
	ByTicket    map[string]int `json:"by_ticket"`
}

func (w *Watchdog) telemetryMarkerPath() string {
	if w == nil || w.config == nil || strings.TrimSpace(w.config.Project.Tracker) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(w.config.Project.Tracker), ".telemetry-maint-last")
}

func (w *Watchdog) shouldRunTelemetryMaintenance(now time.Time) bool {
	p := w.telemetryMarkerPath()
	if p == "" {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return true
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return now.Sub(t) >= 24*time.Hour
}

func (w *Watchdog) markTelemetryMaintenance(now time.Time) {
	p := w.telemetryMarkerPath()
	if p == "" {
		return
	}
	_ = os.WriteFile(p, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func (w *Watchdog) runTelemetryMaintenance(now time.Time) {
	if w == nil || w.events == nil || strings.TrimSpace(w.events.path) == "" {
		return
	}
	if !w.shouldRunTelemetryMaintenance(now) {
		return
	}
	if err := pruneEventsFile(w.events.path, now.Add(-30*24*time.Hour)); err != nil {
		w.log("WARN: telemetry prune failed: %v", err)
	}
	if err := writeDailyRollup(w.events.path, now); err != nil {
		w.log("WARN: telemetry rollup failed: %v", err)
	}
	w.markTelemetryMaintenance(now)
}

func pruneEventsFile(path string, cutoff time.Time) error {
	in, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()

	tmp := path + ".prune.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer out.Close()

	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := sc.Bytes()
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Timestamp.IsZero() || ev.Timestamp.Before(cutoff) {
			continue
		}
		if _, err := out.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeDailyRollup(eventsPath string, now time.Time) error {
	day := now.UTC().Add(-24 * time.Hour).Format("2006-01-02")
	start, _ := time.Parse(time.RFC3339, day+"T00:00:00Z")
	end := start.Add(24 * time.Hour)

	f, err := os.Open(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sum := telemetrySummary{
		DateUTC:     day,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		ByType:      map[string]int{},
		ByTicket:    map[string]int{},
	}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		ts := ev.Timestamp.UTC()
		if ts.Before(start) || !ts.Before(end) {
			continue
		}
		sum.TotalEvents++
		sum.ByType[ev.Type]++
		if strings.TrimSpace(ev.Ticket) != "" {
			sum.ByTicket[ev.Ticket]++
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	rollupDir := filepath.Join(filepath.Dir(eventsPath), "rollups")
	if err := os.MkdirAll(rollupDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(rollupDir, fmt.Sprintf("%s.json", day))
	b, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}

	return pruneOldRollups(rollupDir, now.Add(-365*24*time.Hour))
}

func pruneOldRollups(dir string, cutoff time.Time) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		t, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func listRollupFiles(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}
