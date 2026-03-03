package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubBackend struct{ name string }

func (s stubBackend) Spawn(context.Context, SpawnConfig) (AgentHandle, error) { return AgentHandle{}, errors.New("not implemented") }
func (s stubBackend) IsAlive(AgentHandle) bool                                 { return false }
func (s stubBackend) HasExited(AgentHandle) bool                               { return false }
func (s stubBackend) GetOutput(AgentHandle, int) (string, error)               { return "", nil }
func (s stubBackend) Kill(AgentHandle) error                                    { return nil }
func (s stubBackend) Name() string                                              { return s.name }

func TestRegistryBuild_DefaultAndCodex(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	tests := []struct {
		name        string
		backendType string
		wantName    string
	}{
		{name: "empty type defaults to codex", backendType: "", wantName: TypeCodexTmux},
		{name: "explicit codex", backendType: TypeCodexTmux, wantName: TypeCodexTmux},
		{name: "trimmed codex", backendType: "  codex-tmux  ", wantName: TypeCodexTmux},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := r.Build(tc.backendType, BuildOptions{})
			if err != nil {
				t.Fatalf("build returned error: %v", err)
			}
			if got == nil {
				t.Fatal("expected backend, got nil")
			}
			if got.Name() != tc.wantName {
				t.Fatalf("expected backend name %q, got %q", tc.wantName, got.Name())
			}
		})
	}
}

func TestRegistryBuild_UnknownType(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Build("does-not-exist", BuildOptions{})
	if err == nil {
		t.Fatal("expected error for unknown backend type")
	}
	if !strings.Contains(err.Error(), "unsupported backend type") {
		t.Fatalf("expected unsupported backend type error, got: %v", err)
	}
}

func TestRegistryRegisterAndBuild_CustomBackend(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	if err := r.Register("custom", func(BuildOptions) (AgentBackend, error) {
		return stubBackend{name: "custom"}, nil
	}); err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	got, err := r.Build("custom", BuildOptions{})
	if err != nil {
		t.Fatalf("build returned error: %v", err)
	}
	if got.Name() != "custom" {
		t.Fatalf("expected custom backend, got %q", got.Name())
	}

	if err := r.Register("custom", func(BuildOptions) (AgentBackend, error) {
		return stubBackend{name: "duplicate"}, nil
	}); err == nil {
		t.Fatal("expected duplicate register error")
	}

	if err := r.Register("   ", func(BuildOptions) (AgentBackend, error) {
		return stubBackend{name: "empty"}, nil
	}); err == nil {
		t.Fatal("expected empty backend type error")
	}
}

func TestBuild_UsesDefaultRegistry(t *testing.T) {
	t.Parallel()

	b, err := Build("", BuildOptions{})
	if err != nil {
		t.Fatalf("build returned error: %v", err)
	}
	if b.Name() != TypeCodexTmux {
		t.Fatalf("expected default backend %q, got %q", TypeCodexTmux, b.Name())
	}
}
