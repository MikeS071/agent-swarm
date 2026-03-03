package cmd

import (
	"testing"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
)

func TestBuildBackend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		wantBackend string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "default codex backend",
			cfg: &config.Config{
				Backend: config.BackendConfig{Type: backend.TypeCodexTmux},
			},
			wantBackend: backend.TypeCodexTmux,
		},
		{
			name: "unsupported backend",
			cfg: &config.Config{
				Backend: config.BackendConfig{Type: "unknown"},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := buildBackend(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b == nil {
				t.Fatal("expected backend but got nil")
			}
			if b.Name() != tc.wantBackend {
				t.Fatalf("backend = %q, want %q", b.Name(), tc.wantBackend)
			}
		})
	}
}
