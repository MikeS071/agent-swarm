package tracker

import (
	"encoding/json"
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

func TestLoadSupportsLegacyAndExtendedTicketSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contents    string
		wantType    string
		wantFeature string
		wantProfile string
	}{
		{
			name:        "legacy schema without optional fields",
			contents:    `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[]}}}`,
			wantType:    "",
			wantFeature: "",
			wantProfile: "",
		},
		{
			name:        "extended schema with all optional fields",
			contents:    `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"type":"review","feature":"cache-overhaul","profile":"code-reviewer"}}}`,
			wantType:    "review",
			wantFeature: "cache-overhaul",
			wantProfile: "code-reviewer",
		},
		{
			name:        "extended schema with partial optional fields",
			contents:    `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[],"feature":"cache-overhaul"}}}`,
			wantType:    "",
			wantFeature: "cache-overhaul",
			wantProfile: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeTrackerFile(t, tc.contents)
			tr, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}

			tk, ok := tr.Tickets["t1"]
			if !ok {
				t.Fatal("missing t1")
			}
			if tk.Type != tc.wantType {
				t.Fatalf("type=%q want %q", tk.Type, tc.wantType)
			}
			if tk.Feature != tc.wantFeature {
				t.Fatalf("feature=%q want %q", tk.Feature, tc.wantFeature)
			}
			if tk.Profile != tc.wantProfile {
				t.Fatalf("profile=%q want %q", tk.Profile, tc.wantProfile)
			}
		})
	}
}

func TestSaveToIncludesExtendedTicketSchemaFields(t *testing.T) {
	t.Parallel()

	tr := &Tracker{
		Project: "x",
		Tickets: map[string]Ticket{
			"t1": {
				Status:  "todo",
				Phase:   1,
				Depends: []string{},
				Type:    "review",
				Feature: "cache-overhaul",
				Profile: "code-reviewer",
			},
		},
	}
	path := filepath.Join(t.TempDir(), "tracker.json")
	if err := tr.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	var raw struct {
		Tickets map[string]map[string]any `json:"tickets"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	t1 := raw.Tickets["t1"]
	if t1["type"] != "review" {
		t.Fatalf("type=%v want review", t1["type"])
	}
	if t1["feature"] != "cache-overhaul" {
		t.Fatalf("feature=%v want cache-overhaul", t1["feature"])
	}
	if t1["profile"] != "code-reviewer" {
		t.Fatalf("profile=%v want code-reviewer", t1["profile"])
	}
}

func TestSaveToOmitsEmptyExtendedTicketSchemaFields(t *testing.T) {
	t.Parallel()

	tr := &Tracker{
		Project: "x",
		Tickets: map[string]Ticket{
			"t1": {
				Status:  "todo",
				Phase:   1,
				Depends: []string{},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "tracker.json")
	if err := tr.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	var raw struct {
		Tickets map[string]map[string]any `json:"tickets"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	t1 := raw.Tickets["t1"]
	if _, ok := t1["type"]; ok {
		t.Fatalf("expected type to be omitted, got %v", t1["type"])
	}
	if _, ok := t1["feature"]; ok {
		t.Fatalf("expected feature to be omitted, got %v", t1["feature"])
	}
	if _, ok := t1["profile"]; ok {
		t.Fatalf("expected profile to be omitted, got %v", t1["profile"])
	}
}
