package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/worktree"
)

func TestCleanupWorktreesOlderThan(t *testing.T) {
	repo := initRepo(t)
	m := worktree.New(repo, "", "main")
	if _, err := m.Create("old", "feat/old"); err != nil {
		t.Fatalf("create old: %v", err)
	}
	if _, err := m.Create("new", "feat/new"); err != nil {
		t.Fatalf("create new: %v", err)
	}

	now := time.Now()
	oldTime := now.Add(-72 * time.Hour)
	newTime := now.Add(-1 * time.Hour)
	if err := os.Chtimes(m.Path("old"), oldTime, oldTime); err != nil {
		t.Fatalf("chtime old: %v", err)
	}
	if err := os.Chtimes(m.Path("new"), newTime, newTime); err != nil {
		t.Fatalf("chtime new: %v", err)
	}

	removed, err := CleanupWorktrees(m, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	sort.Strings(removed)
	if len(removed) != 1 || removed[0] != "old" {
		t.Fatalf("unexpected removed ids: %#v", removed)
	}
	if m.Exists("old") {
		t.Fatalf("old should be removed")
	}
	if !m.Exists("new") {
		t.Fatalf("new should remain")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runCmd(t, repo, "git", "init", "-b", "main")
	runCmd(t, repo, "git", "config", "user.email", "test@example.com")
	runCmd(t, repo, "git", "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runCmd(t, repo, "git", "add", "README.md")
	runCmd(t, repo, "git", "commit", "-m", "init")
	return repo
}
