package watchdog

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func TestParseProfileFrontmatterBackend(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "reads backend from frontmatter",
			data: "---\nbackend: claude-code\nmodel: claude-sonnet-4-6\n---\nbody\n",
			want: "claude-code",
		},
		{
			name: "trims quoted backend",
			data: "---\nbackend: \"  openai-api  \"\n---\nbody\n",
			want: "openai-api",
		},
		{
			name: "returns empty without frontmatter",
			data: "backend: claude-code\n",
			want: "",
		},
		{
			name: "returns empty when backend key missing",
			data: "---\nmodel: gpt-5.3-codex\n---\nbody\n",
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseProfileFrontmatterBackend([]byte(tc.data))
			if got != tc.want {
				t.Fatalf("parseProfileFrontmatterBackend() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSpawnBackendType(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "profiles", "code-agent.md"), "---\nbackend: codex-tmux\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "security-reviewer.md"), "---\nbackend: claude-code\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "doc-updater.md"), "---\nname: doc-updater\n---\n")

	w := &Watchdog{
		config: &config.Config{
			Project: config.ProjectConfig{
				Tracker:        filepath.Join(root, "swarm", "tracker.json"),
				PromptDir:      filepath.Join(root, "swarm", "prompts"),
				DefaultProfile: "code-agent",
			},
			Backend: config.BackendConfig{Type: "codex-tmux", Model: "gpt-5.3-codex"},
		},
	}

	cases := []struct {
		name     string
		ticketID string
		ticket   tracker.Ticket
		want     string
	}{
		{
			name:     "explicit profile backend",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{Profile: "security-reviewer"},
			want:     "claude-code",
		},
		{
			name:     "inferred profile backend",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{},
			want:     "claude-code",
		},
		{
			name:     "fallback to default backend type",
			ticketID: "doc-auth",
			ticket:   tracker.Ticket{Profile: "doc-updater"},
			want:     "codex-tmux",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := w.resolveSpawnBackendType(tc.ticketID, tc.ticket)
			if got != tc.want {
				t.Fatalf("resolveSpawnBackendType(%q) = %q, want %q", tc.ticketID, got, tc.want)
			}
		})
	}
}

func TestSpawnTicketRoutesToResolvedBackend(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".agents", "profiles", "security-reviewer.md"), "---\nbackend: alt-backend\nmodel: claude-sonnet-4-6\n---\n")

	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sec-auth.md"), "# sec-auth\n")

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sec-auth": {Status: tracker.StatusTodo, Phase: 1, Profile: "security-reviewer"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:           "proj",
			Repo:           repo,
			BaseBranch:     "main",
			PromptDir:      promptDir,
			Tracker:        trackerPath,
			DefaultProfile: "code-agent",
			MaxAgents:      1,
		},
		Backend: config.BackendConfig{Type: "codex-tmux", Model: "gpt-5.3-codex", Effort: "high"},
	}

	defaultBackend := &fakeBackend{name: "codex-tmux"}
	altBackend := &fakeBackend{
		name: "alt-backend",
		spawnOut: backend.AgentHandle{
			SessionName: "alt-session-sec-auth",
		},
	}

	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, dispatcher.New(cfg, tr), defaultBackend, wt, &fakeNotifier{})
	w.backendFactory = func(backendType string) (backend.AgentBackend, error) {
		switch backendType {
		case "alt-backend":
			return altBackend, nil
		default:
			return defaultBackend, nil
		}
	}

	if err := w.SpawnTicket(context.Background(), "sec-auth"); err != nil {
		t.Fatalf("SpawnTicket() error = %v", err)
	}
	if len(defaultBackend.spawnCalls) != 0 {
		t.Fatalf("default backend should not spawn ticket, got %#v", defaultBackend.spawnCalls)
	}
	if len(altBackend.spawnCalls) != 1 {
		t.Fatalf("alt backend spawn calls = %d, want 1", len(altBackend.spawnCalls))
	}
	if got := tr.Tickets["sec-auth"].SessionBackend; got != "alt-backend" {
		t.Fatalf("SessionBackend = %q, want %q", got, "alt-backend")
	}
	if got := tr.Tickets["sec-auth"].SessionName; got != "alt-session-sec-auth" {
		t.Fatalf("SessionName = %q, want %q", got, "alt-session-sec-auth")
	}
}
