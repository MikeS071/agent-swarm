package backend

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type runCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls     []runCall
	responses map[string]struct {
		out string
		err error
	}
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runCall{name: name, args: append([]string{}, args...)})
	key := name + " " + joinArgs(args)
	if r, ok := f.responses[key]; ok {
		return []byte(r.out), r.err
	}
	return nil, nil
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	s := args[0]
	for _, a := range args[1:] {
		s += " " + a
	}
	return s
}

func TestNewCodexBackendAutoDetectPath(t *testing.T) {
	t.Run("uses lookpath codex first", func(t *testing.T) {
		b := newCodexBackendWithDeps("", true,
			func(s string) (string, error) {
				if s != "codex" {
					t.Fatalf("unexpected lookpath arg: %s", s)
				}
				return "/usr/bin/codex", nil
			},
			func() (string, error) { return "/home/test", nil },
			nil,
			func() time.Time { return time.Unix(100, 0) },
		)
		if b.binary != "/usr/bin/codex" {
			t.Fatalf("expected /usr/bin/codex, got %q", b.binary)
		}
	})

	t.Run("falls back to local bin", func(t *testing.T) {
		b := newCodexBackendWithDeps("", true,
			func(string) (string, error) { return "", errors.New("not found") },
			func() (string, error) { return "/home/test", nil },
			nil,
			func() time.Time { return time.Unix(100, 0) },
		)
		if b.binary != "/home/test/.local/bin/codex" {
			t.Fatalf("unexpected fallback path: %q", b.binary)
		}
	})

	t.Run("honors explicit binary", func(t *testing.T) {
		b := newCodexBackendWithDeps("/opt/codex", false,
			func(string) (string, error) { return "", errors.New("should not run") },
			func() (string, error) { return "", errors.New("should not run") },
			nil,
			func() time.Time { return time.Unix(100, 0) },
		)
		if b.binary != "/opt/codex" {
			t.Fatalf("expected explicit binary, got %q", b.binary)
		}
	})
}

func TestCodexSpawnBuildsTmuxCommand(t *testing.T) {
	fr := &fakeRunner{}
	b := newCodexBackendWithDeps("/usr/bin/codex", true,
		nil,
		nil,
		fr.run,
		func() time.Time { return time.Unix(100, 0) },
	)

	h, err := b.Spawn(context.Background(), SpawnConfig{
		TicketID:   "sw-02",
		WorkDir:    "/repo/worktree",
		PromptFile: "/repo/swarm/prompts/sw-02.md",
		Model:      "gpt-5.3-codex",
		ExtraFlags: []string{"--foo", "bar"},
	})
	if err != nil {
		t.Fatalf("spawn returned err: %v", err)
	}
	if h.SessionName != "swarm-sw-02" {
		t.Fatalf("unexpected session name: %q", h.SessionName)
	}
	if h.StartedAt != time.Unix(100, 0) {
		t.Fatalf("unexpected start time: %v", h.StartedAt)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(fr.calls))
	}
	call := fr.calls[0]
	if call.name != "tmux" {
		t.Fatalf("expected tmux, got %q", call.name)
	}
	wantPrefix := []string{"new-session", "-d", "-s", "swarm-sw-02"}
	if !reflect.DeepEqual(call.args[:4], wantPrefix) {
		t.Fatalf("wrong tmux args prefix: %#v", call.args)
	}
	if got := call.args[4]; got == "" {
		t.Fatalf("expected shell command")
	}
	if h.PID != 0 {
		t.Fatalf("expected PID 0 for now, got %d", h.PID)
	}
}

func TestCodexLifecycleAndOutput(t *testing.T) {
	fr := &fakeRunner{responses: map[string]struct {
		out string
		err error
	}{
		"tmux has-session -t swarm-sw-02":                 {out: "", err: nil},
		"tmux list-panes -t swarm-sw-02 -F #{pane_pid}":   {out: "4242\n", err: nil},
		"ps -p 4242":                                      {out: "", err: errors.New("not running")},
		"tmux capture-pane -t swarm-sw-02 -p -S -50":      {out: "line1\nline2\n", err: nil},
		"tmux kill-session -t swarm-sw-02":                {out: "", err: nil},
		"tmux list-sessions -F #{session_name}":           {out: "swarm-sw-01\nother\nswarm-sw-02\n", err: nil},
	}}
	b := newCodexBackendWithDeps("/usr/bin/codex", true, nil, nil, fr.run, time.Now)
	h := AgentHandle{SessionName: "swarm-sw-02"}

	if !b.IsAlive(h) {
		t.Fatalf("expected alive")
	}
	if !b.HasExited(h) {
		t.Fatalf("expected exited")
	}
	out, err := b.GetOutput(h, 50)
	if err != nil {
		t.Fatalf("get output err: %v", err)
	}
	if out != "line1\nline2\n" {
		t.Fatalf("unexpected output: %q", out)
	}
	if err := b.Kill(h); err != nil {
		t.Fatalf("kill err: %v", err)
	}
	sessions, err := b.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions err: %v", err)
	}
	if !reflect.DeepEqual(sessions, []string{"swarm-sw-01", "swarm-sw-02"}) {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}
