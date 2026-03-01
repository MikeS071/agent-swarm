package tracker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTrackerFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "tracker.json")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write tracker: %v", err)
	}
	return p
}

func sampleTracker() *Tracker {
	return &Tracker{
		Project: "test",
		Tickets: map[string]Ticket{
			"a": {Status: "done", Phase: 1, Depends: []string{}},
			"b": {Status: "todo", Phase: 1, Depends: []string{"a"}},
			"c": {Status: "todo", Phase: 2, Depends: []string{"b"}},
			"d": {Status: "failed", Phase: 1, Depends: []string{}},
		},
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Parallel()
	tr := sampleTracker()
	path := filepath.Join(t.TempDir(), "tracker.json")
	if err := tr.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Project != tr.Project {
		t.Fatalf("project mismatch: got %q want %q", got.Project, tr.Project)
	}
	if len(got.Tickets) != len(tr.Tickets) {
		t.Fatalf("tickets count mismatch: got %d want %d", len(got.Tickets), len(tr.Tickets))
	}
}

func TestSetStatus(t *testing.T) {
	t.Parallel()
	tr := sampleTracker()
	if err := tr.SetStatus("b", "running"); err != nil {
		t.Fatalf("SetStatus running: %v", err)
	}
	if tr.Tickets["b"].Status != "running" {
		t.Fatalf("status not updated: %q", tr.Tickets["b"].Status)
	}

	if err := tr.SetStatus("missing", "done"); err == nil {
		t.Fatal("expected error for missing ticket")
	}
	if err := tr.SetStatus("b", "bad-status"); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestActivePhaseAndSpawnable(t *testing.T) {
	t.Parallel()
	tr := sampleTracker()

	if got := tr.ActivePhase(); got != 1 {
		t.Fatalf("ActivePhase() = %d, want 1", got)
	}
	spawnable := tr.GetSpawnable()
	want := []string{"b"}
	if !reflect.DeepEqual(spawnable, want) {
		t.Fatalf("GetSpawnable() = %v, want %v", spawnable, want)
	}
}

func TestGetByPhase(t *testing.T) {
	t.Parallel()
	tr := sampleTracker()
	phase1 := tr.GetByPhase(1)
	if len(phase1) != 3 {
		t.Fatalf("GetByPhase(1) len=%d want 3", len(phase1))
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	tr := sampleTracker()
	stats := tr.Stats()
	if stats.Total != 4 {
		t.Fatalf("total=%d want 4", stats.Total)
	}
	if stats.Done != 1 || stats.Todo != 2 || stats.Failed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestDependencyOrderTopoSort(t *testing.T) {
	t.Parallel()
	tr := &Tracker{
		Project: "topo",
		Tickets: map[string]Ticket{
			"a": {Depends: []string{}},
			"b": {Depends: []string{"a"}},
			"c": {Depends: []string{"a"}},
			"d": {Depends: []string{"b", "c"}},
		},
	}
	order := tr.DependencyOrder()
	idx := map[string]int{}
	for i, id := range order {
		idx[id] = i
	}

	if !(idx["a"] < idx["b"] && idx["a"] < idx["c"] && idx["b"] < idx["d"] && idx["c"] < idx["d"]) {
		t.Fatalf("invalid topo order: %v", order)
	}
}

func TestLoadMissingFileFails(t *testing.T) {
	t.Parallel()
	_, err := Load("/not/here/tracker.json")
	if err == nil {
		t.Fatal("expected load error for missing tracker")
	}
}

func TestLoadFromFixture(t *testing.T) {
	t.Parallel()
	path := writeTrackerFile(t, `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[]}}}`)
	tr, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if tr.Project != "x" {
		t.Fatalf("project=%q want x", tr.Project)
	}
}
