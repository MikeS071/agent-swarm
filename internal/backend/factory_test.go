package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubBackend struct{ name string }

func (s stubBackend) Spawn(context.Context, SpawnConfig) (AgentHandle, error) {
	return AgentHandle{}, errors.New("not implemented")
}
func (s stubBackend) IsAlive(AgentHandle) bool                   { return false }
func (s stubBackend) HasExited(AgentHandle) bool                 { return false }
func (s stubBackend) GetOutput(AgentHandle, int) (string, error) { return "", nil }
func (s stubBackend) Kill(AgentHandle) error                     { return nil }
func (s stubBackend) Name() string                               { return s.name }

func TestRegistryBuild_DefaultAndCodex(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cases := []struct {
		name        string
		backendType string
		wantName    string
	}{
		{name: "empty defaults to codex", backendType: "", wantName: TypeCodexTmux},
		{name: "explicit codex", backendType: TypeCodexTmux, wantName: TypeCodexTmux},
		{name: "trimmed codex", backendType: "  codex-tmux  ", wantName: TypeCodexTmux},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := r.Build(tc.backendType, BuildOptions{})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if b == nil {
				t.Fatal("Build() returned nil backend")
			}
			if b.Name() != tc.wantName {
				t.Fatalf("backend name = %q, want %q", b.Name(), tc.wantName)
			}
		})
	}
}

func TestRegistryBuildUnknownType(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Build("does-not-exist", BuildOptions{})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unsupported backend type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistryRegisterCustomBackend(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.Register("custom", func(BuildOptions) (AgentBackend, error) {
		return stubBackend{name: "custom"}, nil
	}); err != nil {
		t.Fatalf("register error = %v", err)
	}

	b, err := r.Build("custom", BuildOptions{})
	if err != nil {
		t.Fatalf("build error = %v", err)
	}
	if b.Name() != "custom" {
		t.Fatalf("backend name = %q, want custom", b.Name())
	}

	if err := r.Register("custom", func(BuildOptions) (AgentBackend, error) {
		return stubBackend{name: "dupe"}, nil
	}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestBuildUsesDefaultRegistry(t *testing.T) {
	t.Parallel()

	b, err := Build("", BuildOptions{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if b.Name() != TypeCodexTmux {
		t.Fatalf("backend name = %q, want %q", b.Name(), TypeCodexTmux)
	}
}
