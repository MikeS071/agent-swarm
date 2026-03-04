package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMaterializeAgentContextCopiesSourcesAndWritesManifest(t *testing.T) {
	repo := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	writeContextFile(t, filepath.Join(repo, ".codex", "rules", "base.md"), "base rule\n")
	writeContextFile(t, filepath.Join(repo, ".codex", "rules", "backend.md"), "backend rule\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "profiles", "backend.md"), "profile\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "roles", "backend.yaml"), "name: backend\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "skills", "testing", "SKILL.md"), "testing skill\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "skills", "docs", "SKILL.md"), "docs skill\n")

	m := New(repo, "", "main")
	createdAt := time.Date(2026, 3, 4, 10, 33, 0, 0, time.UTC)
	manifestPath, manifest, err := m.MaterializeAgentContext(AgentContextOptions{
		TicketID:     "sw-01",
		WorktreePath: worktreePath,
		Role:         "backend",
		RunID:        "run-2026-03-04T10-32-00Z",
		CreatedAt:    createdAt,
	})
	if err != nil {
		t.Fatalf("materialize context: %v", err)
	}

	wantManifestPath := filepath.Join(worktreePath, ".agent-context", "context-manifest.json")
	if manifestPath != wantManifestPath {
		t.Fatalf("manifest path = %q, want %q", manifestPath, wantManifestPath)
	}
	if manifest.Ticket != "sw-01" {
		t.Fatalf("manifest ticket = %q", manifest.Ticket)
	}
	if manifest.RunID != "run-2026-03-04T10-32-00Z" {
		t.Fatalf("manifest runId = %q", manifest.RunID)
	}
	if manifest.Role != "backend" {
		t.Fatalf("manifest role = %q", manifest.Role)
	}
	if manifest.CreatedAt != "2026-03-04T10:33:00Z" {
		t.Fatalf("manifest createdAt = %q", manifest.CreatedAt)
	}

	wantSourcePaths := []string{
		".agents/profiles/backend.md",
		".agents/roles/backend.yaml",
		".agents/skills/docs/SKILL.md",
		".agents/skills/testing/SKILL.md",
		".codex/rules/backend.md",
		".codex/rules/base.md",
	}
	gotPaths := make([]string, 0, len(manifest.Sources))
	for _, src := range manifest.Sources {
		gotPaths = append(gotPaths, src.Path)
	}
	if !reflect.DeepEqual(gotPaths, wantSourcePaths) {
		t.Fatalf("manifest source paths = %#v, want %#v", gotPaths, wantSourcePaths)
	}

	baseHash := sha256.Sum256([]byte("base rule\n"))
	wantBaseHash := hex.EncodeToString(baseHash[:])
	foundBase := false
	for _, src := range manifest.Sources {
		if src.Path == ".codex/rules/base.md" {
			foundBase = true
			if src.SHA256 != wantBaseHash {
				t.Fatalf("base hash = %q, want %q", src.SHA256, wantBaseHash)
			}
		}
	}
	if !foundBase {
		t.Fatalf("missing .codex/rules/base.md in manifest")
	}

	checks := []string{
		filepath.Join(worktreePath, ".agent-context", "rules", "base.md"),
		filepath.Join(worktreePath, ".agent-context", "rules", "backend.md"),
		filepath.Join(worktreePath, ".agent-context", "profiles", "backend.md"),
		filepath.Join(worktreePath, ".agent-context", "roles", "backend.yaml"),
		filepath.Join(worktreePath, ".agent-context", "skills", "docs", "SKILL.md"),
		filepath.Join(worktreePath, ".agent-context", "skills", "testing", "SKILL.md"),
	}
	for _, p := range checks {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected copied snapshot file %s: %v", p, err)
		}
	}

	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest file: %v", err)
	}
	var onDisk ContextManifest
	if err := json.Unmarshal(body, &onDisk); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if !reflect.DeepEqual(onDisk, manifest) {
		t.Fatalf("on-disk manifest mismatch\n got=%#v\nwant=%#v", onDisk, manifest)
	}
}

func TestMaterializeAgentContextErrorsWhenRoleProfileMissing(t *testing.T) {
	repo := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	writeContextFile(t, filepath.Join(repo, ".codex", "rules", "base.md"), "base rule\n")

	m := New(repo, "", "main")
	_, _, err := m.MaterializeAgentContext(AgentContextOptions{
		TicketID:     "sw-02",
		WorktreePath: worktreePath,
		Role:         "backend",
		RunID:        "run-02",
		CreatedAt:    time.Date(2026, 3, 4, 10, 35, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatalf("expected error when role profile is missing")
	}
	if !strings.Contains(err.Error(), "profile") {
		t.Fatalf("expected profile-related error, got %v", err)
	}
}

func TestMaterializeAgentContextDeterministicManifestOrdering(t *testing.T) {
	repo := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	writeContextFile(t, filepath.Join(repo, ".codex", "rules", "z-last.md"), "z\n")
	writeContextFile(t, filepath.Join(repo, ".codex", "rules", "a-first.md"), "a\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "profiles", "backend.md"), "profile\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "skills", "zzz", "SKILL.md"), "zzz\n")
	writeContextFile(t, filepath.Join(repo, ".agents", "skills", "aaa", "SKILL.md"), "aaa\n")

	m := New(repo, "", "main")
	opts := AgentContextOptions{
		TicketID:     "sw-03",
		WorktreePath: worktreePath,
		Role:         "backend",
		RunID:        "run-03",
		CreatedAt:    time.Date(2026, 3, 4, 10, 40, 0, 0, time.UTC),
	}

	manifestPath, first, err := m.MaterializeAgentContext(opts)
	if err != nil {
		t.Fatalf("first materialize context: %v", err)
	}
	firstBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(worktreePath, ".agent-context")); err != nil {
		t.Fatalf("reset .agent-context dir: %v", err)
	}

	manifestPath, second, err := m.MaterializeAgentContext(opts)
	if err != nil {
		t.Fatalf("second materialize context: %v", err)
	}
	secondBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read second manifest: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("manifest structs differ between runs\nfirst=%#v\nsecond=%#v", first, second)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("manifest bytes differ between identical runs\nfirst=%s\nsecond=%s", string(firstBytes), string(secondBytes))
	}
}

func writeContextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}
