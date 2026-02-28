package progress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
)

func TestParseMarker(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantX  int
		wantN  int
		isNil  bool
	}{
		{name: "none", input: "hello", isNil: true},
		{name: "single", input: "PROGRESS: 2/5", wantX: 2, wantN: 5},
		{name: "last wins", input: "PROGRESS: 1/3\nfoo\nPROGRESS: 2/3", wantX: 2, wantN: 3},
		{name: "spaces", input: "  PROGRESS:   4 / 9  ", wantX: 4, wantN: 9},
		{name: "bad total", input: "PROGRESS: 2/0", isNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := ParseMarker(tc.input)
			if tc.isNil {
				if m != nil {
					t.Fatalf("expected nil marker, got %#v", m)
				}
				return
			}
			if m == nil {
				t.Fatalf("expected marker")
			}
			if m.Done != tc.wantX || m.Total != tc.wantN {
				t.Fatalf("unexpected marker: %#v", m)
			}
		})
	}
}

func TestInferHeuristic(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		runtime time.Duration
		want    int
	}{
		{name: "fresh", output: "", runtime: 10 * time.Second, want: 5},
		{name: "file changes", output: "write file main.go\ncreated src/app.ts", runtime: 1 * time.Minute, want: 30},
		{name: "thinking", output: "thinking hard\nwrite file", runtime: 2 * time.Minute, want: 50},
		{name: "build", output: "go test ./...\nPASS", runtime: 3 * time.Minute, want: 70},
		{name: "commit", output: "git commit -m done", runtime: 4 * time.Minute, want: 90},
		{name: "push", output: "git push origin feat/sw-02", runtime: 5 * time.Minute, want: 95},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InferHeuristic(tc.output, tc.runtime)
			if got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}

type fakeBackend struct {
	output string
	err    error
	alive  bool
	exited bool
}

func (f fakeBackend) Spawn(context.Context, backend.SpawnConfig) (backend.AgentHandle, error) {
	return backend.AgentHandle{}, errors.New("not used")
}
func (f fakeBackend) IsAlive(backend.AgentHandle) bool                            { return f.alive }
func (f fakeBackend) HasExited(backend.AgentHandle) bool                          { return f.exited }
func (f fakeBackend) GetOutput(backend.AgentHandle, int) (string, error)         { return f.output, f.err }
func (f fakeBackend) Kill(backend.AgentHandle) error                              { return nil }
func (f fakeBackend) Name() string                                                { return "fake" }

func TestGetProgress(t *testing.T) {
	h := backend.AgentHandle{SessionName: "swarm-sw-02", StartedAt: time.Now().Add(-2 * time.Minute)}

	t.Run("uses marker when present", func(t *testing.T) {
		p := GetProgress(h, fakeBackend{output: "x\nPROGRESS: 2/4\n", alive: true}, 4)
		if p.Source != "marker" || p.Progress != 50 || p.TasksDone != 2 || p.TasksTotal != 4 {
			t.Fatalf("unexpected progress: %#v", p)
		}
		if p.Status != "running" {
			t.Fatalf("unexpected status: %s", p.Status)
		}
	})

	t.Run("falls back heuristic", func(t *testing.T) {
		p := GetProgress(h, fakeBackend{output: "go test ./...", alive: true}, 8)
		if p.Source != "heuristic" || p.Progress != 70 {
			t.Fatalf("unexpected progress: %#v", p)
		}
		if p.TasksTotal != 8 {
			t.Fatalf("expected prompt tasks total in fallback")
		}
	})

	t.Run("done status if exited", func(t *testing.T) {
		p := GetProgress(h, fakeBackend{output: "git push", alive: false, exited: true}, 3)
		if p.Status != "done" || p.Progress != 100 {
			t.Fatalf("unexpected progress: %#v", p)
		}
	})

	t.Run("failed status on output error", func(t *testing.T) {
		p := GetProgress(h, fakeBackend{err: errors.New("boom"), alive: false}, 3)
		if p.Status != "failed" || p.Progress != 0 {
			t.Fatalf("unexpected progress: %#v", p)
		}
	})
}
