package watchdog

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func TestSelectProfileName(t *testing.T) {
	t.Parallel()

	w := &Watchdog{
		config: &config.Config{
			Project: config.ProjectConfig{
				DefaultProfile: "code-agent",
			},
		},
	}

	cases := []struct {
		name     string
		ticketID string
		ticket   tracker.Ticket
		want     string
	}{
		{
			name:     "explicit profile overrides inferred and default",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{Profile: "doc-updater"},
			want:     "doc-updater",
		},
		{
			name:     "prefix inferred profile when explicit missing",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{},
			want:     "security-reviewer",
		},
		{
			name:     "default profile fallback when no prefix mapping",
			ticketID: "sw-01",
			ticket:   tracker.Ticket{},
			want:     "code-agent",
		},
		{
			name:     "empty when no explicit inferred or default profile",
			ticketID: "sw-01",
			ticket:   tracker.Ticket{},
			want:     "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			origDefault := w.config.Project.DefaultProfile
			if tc.name == "empty when no explicit inferred or default profile" {
				w.config.Project.DefaultProfile = ""
			}
			got := w.selectProfileName(tc.ticketID, tc.ticket)
			w.config.Project.DefaultProfile = origDefault
			if got != tc.want {
				t.Fatalf("selectProfileName(%q) = %q, want %q", tc.ticketID, got, tc.want)
			}
		})
	}
}

func TestParseProfileFrontmatterModel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "reads unquoted model from frontmatter",
			data: "---\nname: code-agent\nmodel: gpt-5.3-codex\n---\nbody\n",
			want: "gpt-5.3-codex",
		},
		{
			name: "reads quoted model and trims whitespace",
			data: "---\nmodel: \"  claude-opus-4-6  \"\n---\nbody\n",
			want: "claude-opus-4-6",
		},
		{
			name: "returns empty when no frontmatter",
			data: "# profile\nmodel: gpt-5.3-codex\n",
			want: "",
		},
		{
			name: "returns empty when frontmatter has no model",
			data: "---\nname: planner\nmode: Research\n---\nbody\n",
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseProfileFrontmatterModel([]byte(tc.data))
			if got != tc.want {
				t.Fatalf("parseProfileFrontmatterModel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSpawnModel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "profiles", "code-agent.md"), "---\nmodel: gpt-5.2\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "security-reviewer.md"), "---\nmodel: sonnet\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "doc-updater.md"), "---\nname: doc-updater\n---\n")

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Tracker:        filepath.Join(root, "swarm", "tracker.json"),
			PromptDir:      filepath.Join(root, "swarm", "prompts"),
			DefaultProfile: "code-agent",
		},
		Backend: config.BackendConfig{
			Type:   "codex-tmux",
			Model:  "gpt-5.3-codex",
			Effort: "high",
		},
	}
	w := &Watchdog{config: cfg}

	cases := []struct {
		name       string
		ticketID   string
		ticket     tracker.Ticket
		backendTyp string
		want       string
	}{
		{
			name:     "uses codex profile model when compatible",
			ticketID: "feat-auth-01",
			ticket:   tracker.Ticket{Profile: "code-agent"},
			want:     "gpt-5.2",
		},
		{
			name:     "falls back for non-codex profile model on codex backend",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{Profile: "security-reviewer"},
			want:     "gpt-5.3-codex",
		},
		{
			name:     "falls back when profile file is missing",
			ticketID: "feat-auth-02",
			ticket:   tracker.Ticket{Profile: "missing-profile"},
			want:     "gpt-5.3-codex",
		},
		{
			name:     "falls back when frontmatter has no model",
			ticketID: "doc-auth",
			ticket:   tracker.Ticket{Profile: "doc-updater"},
			want:     "gpt-5.3-codex",
		},
		{
			name:     "uses inferred profile when explicit profile absent",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{},
			want:     "gpt-5.3-codex",
		},
		{
			name:       "allows non-codex model for non-codex backend",
			ticketID:   "sec-auth",
			ticket:     tracker.Ticket{},
			backendTyp: "claude-cli",
			want:       "sonnet",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			origType := w.config.Backend.Type
			if tc.backendTyp != "" {
				w.config.Backend.Type = tc.backendTyp
			}
			got := w.resolveSpawnModel(tc.ticketID, tc.ticket)
			w.config.Backend.Type = origType
			if got != tc.want {
				t.Fatalf("resolveSpawnModel(%q) = %q, want %q", tc.ticketID, got, tc.want)
			}
		})
	}
}

func TestSpawnTicketUsesResolvedModelFromProfileFrontmatter(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".agents", "profiles", "code-agent.md"), "---\nmodel: gpt-5.2\n---\n")

	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-01.md"), "# sw-01\n")

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
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
		Backend: config.BackendConfig{
			Type:   "codex-tmux",
			Model:  "gpt-5.3-codex",
			Effort: "high",
		},
	}

	be := &fakeBackend{}
	d := dispatcher.New(cfg, tr)
	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, d, be, wt, &fakeNotifier{})

	if err := w.SpawnTicket(context.Background(), "sw-01"); err != nil {
		t.Fatalf("SpawnTicket() error = %v", err)
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(be.spawnCalls))
	}
	if got := be.spawnCalls[0].Model; got != "gpt-5.2" {
		t.Fatalf("spawn model = %q, want %q", got, "gpt-5.2")
	}
}
