package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func TestSelectProfileName(t *testing.T) {
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
			name:     "empty when explicit missing",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{},
			want:     "",
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
			w := &Watchdog{config: &config.Config{}}
			got := w.selectProfileName(tc.ticketID, tc.ticket)
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
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "profiles", "code-agent.md"), "---\nmodel: gpt-5.2\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "security-reviewer.md"), "---\nmodel: sonnet\n---\n")
	writeFile(t, filepath.Join(root, ".agents", "profiles", "doc-updater.md"), "---\nname: doc-updater\n---\n")

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
			name:     "falls back when explicit profile absent",
			ticketID: "sec-auth",
			ticket:   tracker.Ticket{},
			want:     "gpt-5.3-codex",
		},
		{
			name:       "non-codex backend without explicit profile still uses default model",
			ticketID:   "sec-auth",
			ticket:     tracker.Ticket{},
			backendTyp: "claude-cli",
			want:       "gpt-5.3-codex",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			backendType := "codex-tmux"
			if tc.backendTyp != "" {
				backendType = tc.backendTyp
			}
			w := &Watchdog{
				config: &config.Config{
					Project: config.ProjectConfig{
						Tracker:   filepath.Join(root, "swarm", "tracker.json"),
						PromptDir: filepath.Join(root, "swarm", "prompts"),
					},
					Backend: config.BackendConfig{
						Type:   backendType,
						Model:  "gpt-5.3-codex",
						Effort: "high",
					},
				},
			}
			got := w.resolveSpawnModel(tc.ticketID, tc.ticket)
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
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
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

func TestSpawnTicketMaterializesAgentContextAndLogsManifestPath(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".codex", "rules", "base.md"), "base\n")
	writeFile(t, filepath.Join(repo, ".codex", "rules", "code-agent.md"), "code-agent rule\n")
	writeFile(t, filepath.Join(repo, ".agents", "profiles", "code-agent.md"), "---\nmodel: gpt-5.2\n---\n")
	writeFile(t, filepath.Join(repo, ".agents", "skills", "tdd-workflow", "SKILL.md"), "TDD\n")

	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-11.md"), "# sw-11\n")

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-11": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent", RunID: "run-11"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
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

	if err := w.SpawnTicket(context.Background(), "sw-11"); err != nil {
		t.Fatalf("SpawnTicket() error = %v", err)
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(be.spawnCalls))
	}

	call := be.spawnCalls[0]
	wantManifestPath := filepath.Join(call.WorkDir, ".agent-context", "context-manifest.json")
	if call.ContextManifestPath != wantManifestPath {
		t.Fatalf("context manifest path = %q, want %q", call.ContextManifestPath, wantManifestPath)
	}
	if _, err := os.Stat(wantManifestPath); err != nil {
		t.Fatalf("manifest missing at %s: %v", wantManifestPath, err)
	}

	b, err := os.ReadFile(wantManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest["role"] != "code-agent" {
		t.Fatalf("manifest role = %v", manifest["role"])
	}

	eventsPath := filepath.Join(filepath.Dir(trackerPath), "events.jsonl")
	eventsRaw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(eventsRaw)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected at least one event line")
	}
	var last Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("unmarshal last event: %v", err)
	}
	if last.Type != "ticket_spawned" {
		t.Fatalf("last event type = %q, want ticket_spawned", last.Type)
	}
	gotPath, _ := last.Data["context_manifest_path"].(string)
	if gotPath != wantManifestPath {
		t.Fatalf("event context_manifest_path = %q, want %q", gotPath, wantManifestPath)
	}
}

func TestSpawnTicketFailsWhenContextProfileSourceMissing(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".codex", "rules", "base.md"), "base\n")

	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-12.md"), "# sw-12\n")

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-12": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:       "proj",
			Repo:       repo,
			BaseBranch: "main",
			PromptDir:  promptDir,
			Tracker:    trackerPath,
			MaxAgents:  1,
		},
		Backend: config.BackendConfig{Type: "codex-tmux", Model: "gpt-5.3-codex", Effort: "high"},
	}

	be := &fakeBackend{}
	d := dispatcher.New(cfg, tr)
	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, d, be, wt, &fakeNotifier{})

	err := w.SpawnTicket(context.Background(), "sw-12")
	if err == nil {
		t.Fatalf("expected SpawnTicket error when role profile source is missing")
	}
	if !strings.Contains(err.Error(), "profile") {
		t.Fatalf("expected profile-related error, got %v", err)
	}
	if len(be.spawnCalls) != 0 {
		t.Fatalf("expected no backend spawn call on context snapshot error")
	}
}

func TestSpawnTicketWithoutRoleStillCreatesContextManifest(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, filepath.Join(repo, ".codex", "rules", "base.md"), "base\n")
	writeFile(t, filepath.Join(repo, ".agents", "skills", "tdd-workflow", "SKILL.md"), "TDD\n")

	promptDir := filepath.Join(repo, "swarm", "prompts")
	writeFile(t, filepath.Join(promptDir, "sw-13.md"), "# sw-13\n")

	trackerPath := filepath.Join(repo, "swarm", "tracker.json")
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"sw-13": {Status: tracker.StatusTodo, Phase: 1},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	cfg := &config.Config{
		Project: config.ProjectConfig{
			Name:                "proj",
			Repo:                repo,
			BaseBranch:          "main",
			PromptDir:           promptDir,
			Tracker:             trackerPath,
			MaxAgents:           1,
			RequireExplicitRole: false,
		},
		Backend: config.BackendConfig{Type: "codex-tmux", Model: "gpt-5.3-codex", Effort: "high"},
	}

	be := &fakeBackend{}
	d := dispatcher.New(cfg, tr)
	wt := worktree.New(repo, filepath.Join(t.TempDir(), "wts"), "main")
	w := New(cfg, tr, d, be, wt, &fakeNotifier{})

	if err := w.SpawnTicket(context.Background(), "sw-13"); err != nil {
		t.Fatalf("SpawnTicket() error = %v", err)
	}
	if len(be.spawnCalls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(be.spawnCalls))
	}

	manifestPath := filepath.Join(be.spawnCalls[0].WorkDir, ".agent-context", "context-manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if _, ok := manifest["role"]; ok {
		t.Fatalf("expected empty role to be omitted from manifest, got %v", manifest["role"])
	}
}
