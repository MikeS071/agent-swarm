package feature

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		feature    string
		setup      func(t *testing.T, s *Store)
		wantErr    bool
		wantErrSub string
	}{
		{
			name:    "happy path",
			feature: "cache-overhaul",
		},
		{
			name:    "invalid name",
			feature: "../bad",
			wantErr: true,
		},
		{
			name:    "duplicate feature",
			feature: "cache-overhaul",
			setup: func(t *testing.T, s *Store) {
				t.Helper()
				if _, err := s.Add("cache-overhaul"); err != nil {
					t.Fatalf("pre-add feature: %v", err)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "swarm", "features")
			s := NewStore(root)
			if tt.setup != nil {
				tt.setup(t, s)
			}

			f, err := s.Add(tt.feature)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Add(%q) expected error", tt.feature)
				}
				if tt.wantErrSub != "" && !contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("Add(%q) error %q does not contain %q", tt.feature, err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add(%q) error = %v", tt.feature, err)
			}
			if f.Name != tt.feature {
				t.Fatalf("name = %q, want %q", f.Name, tt.feature)
			}
			if f.State != StateDraft {
				t.Fatalf("state = %q, want %q", f.State, StateDraft)
			}

			path := filepath.Join(root, tt.feature, MetadataFileName)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("feature metadata file missing at %s: %v", path, err)
			}
		})
	}
}

func TestStoreAdvance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		from       State
		target     State
		wantErr    bool
		wantErrSub string
	}{
		{
			name:   "happy path adjacent transition",
			from:   StateDraft,
			target: StatePRDReview,
		},
		{
			name:       "cannot skip states",
			from:       StateDraft,
			target:     StateArchReview,
			wantErr:    true,
			wantErrSub: "invalid transition",
		},
		{
			name:       "invalid target state",
			from:       StateDraft,
			target:     State("not-a-state"),
			wantErr:    true,
			wantErrSub: "invalid state",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "swarm", "features")
			s := NewStore(root)
			if _, err := s.Add("cache-overhaul"); err != nil {
				t.Fatalf("Add: %v", err)
			}
			if tt.from != StateDraft {
				if _, err := s.Advance("cache-overhaul", tt.from); err != nil {
					t.Fatalf("Advance to setup state %q: %v", tt.from, err)
				}
			}

			got, err := s.Advance("cache-overhaul", tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Advance expected error")
				}
				if tt.wantErrSub != "" && !contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("Advance error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Advance error = %v", err)
			}
			if got.State != tt.target {
				t.Fatalf("state = %q, want %q", got.State, tt.target)
			}
		})
	}
}

func TestStoreLifecyclePathToComplete(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "swarm", "features")
	s := NewStore(root)

	if _, err := s.Add("cache-overhaul"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	path := []State{
		StatePRDReview,
		StateArchReview,
		StateSpecReview,
		StatePlanned,
		StateBuilding,
		StatePostBuild,
		StateComplete,
	}

	for _, st := range path {
		if _, err := s.Advance("cache-overhaul", st); err != nil {
			t.Fatalf("Advance to %q: %v", st, err)
		}
	}

	loaded, err := s.Get("cache-overhaul")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.State != StateComplete {
		t.Fatalf("final state = %q, want %q", loaded.State, StateComplete)
	}
}

func TestStoreList(t *testing.T) {
	t.Parallel()

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "swarm", "features")
		s := NewStore(root)
		got, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("len(list) = %d, want 0", len(got))
		}
	})

	t.Run("sorted by name", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "swarm", "features")
		s := NewStore(root)
		if _, err := s.Add("b-feature"); err != nil {
			t.Fatalf("add b: %v", err)
		}
		if _, err := s.Add("a-feature"); err != nil {
			t.Fatalf("add a: %v", err)
		}

		list, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		names := make([]string, 0, len(list))
		for _, f := range list {
			names = append(names, f.Name)
		}
		want := []string{"a-feature", "b-feature"}
		if !reflect.DeepEqual(names, want) {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	})

	t.Run("malformed metadata returns error", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "swarm", "features")
		if err := os.MkdirAll(filepath.Join(root, "bad"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(root, "bad", MetadataFileName)
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("write malformed metadata: %v", err)
		}

		s := NewStore(root)
		if _, err := s.List(); err == nil {
			t.Fatal("expected error for malformed metadata")
		}
	})
}

func TestStoreSave(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "swarm", "features")
	s := NewStore(root)
	f, err := s.Add("cache-overhaul")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	f.Tickets = []string{"cch-01"}
	if err := s.Save(f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(root, "cache-overhaul", MetadataFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded Feature
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(loaded.Tickets, []string{"cch-01"}) {
		t.Fatalf("tickets = %#v, want %#v", loaded.Tickets, []string{"cch-01"})
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
