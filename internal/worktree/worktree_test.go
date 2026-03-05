package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndRemoveWorktree(t *testing.T) {
	repo := initRepo(t)

	m := New(repo, "", "main")
	path, err := m.Create("sw-04", "feat/sw-04")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if got := m.Path("sw-04"); got != path {
		t.Fatalf("unexpected path: got %q want %q", got, path)
	}
	if !m.Exists("sw-04") {
		t.Fatalf("expected worktree to exist")
	}

	if err := m.Remove("sw-04"); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if m.Exists("sw-04") {
		t.Fatalf("expected worktree to not exist after remove")
	}
}

func TestHasCommits(t *testing.T) {
	repo := initRepo(t)
	m := New(repo, "", "main")
	wtPath, err := m.Create("sw-04", "feat/sw-04")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	has, sha, err := m.HasCommits("sw-04", "main")
	if err != nil {
		t.Fatalf("has commits before changes: %v", err)
	}
	if has {
		t.Fatalf("expected no commits before making one")
	}
	if sha != "" {
		t.Fatalf("expected empty sha for no commits, got %q", sha)
	}

	writeFile(t, filepath.Join(wtPath, "note.txt"), "hello\n")
	runGit(t, wtPath, "add", "note.txt")
	runGit(t, wtPath, "commit", "-m", "add note")

	has, sha, err = m.HasCommits("sw-04", "main")
	if err != nil {
		t.Fatalf("has commits after commit: %v", err)
	}
	if !has {
		t.Fatalf("expected commits to be detected")
	}
	if len(sha) < 7 {
		t.Fatalf("expected non-empty sha, got %q", sha)
	}
}

func TestListParsesPorcelain(t *testing.T) {
	repo := initRepo(t)
	m := New(repo, "", "main")
	_, err := m.Create("sw-04", "feat/sw-04")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(list) < 2 {
		t.Fatalf("expected at least 2 worktrees, got %d", len(list))
	}

	found := false
	for _, wt := range list {
		if strings.HasSuffix(wt.Path, "/sw-04") {
			found = true
			if wt.Branch != "feat/sw-04" {
				t.Fatalf("unexpected branch for sw-04 worktree: %q", wt.Branch)
			}
		}
	}
	if !found {
		t.Fatalf("did not find sw-04 worktree in list")
	}
}

func TestCleanupOlderThan(t *testing.T) {
	repo := initRepo(t)
	m := New(repo, "", "main")
	_, err := m.Create("old", "feat/old")
	if err != nil {
		t.Fatalf("create old worktree: %v", err)
	}
	_, err = m.Create("new", "feat/new")
	if err != nil {
		t.Fatalf("create new worktree: %v", err)
	}

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	newTime := now.Add(-30 * time.Minute)
	if err := os.Chtimes(m.Path("old"), oldTime, oldTime); err != nil {
		t.Fatalf("set old ctimes: %v", err)
	}
	if err := os.Chtimes(m.Path("new"), newTime, newTime); err != nil {
		t.Fatalf("set new ctimes: %v", err)
	}

	removed, err := m.CleanupOlderThan(24 * time.Hour)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(removed) != 1 || removed[0] != "old" {
		t.Fatalf("unexpected removed list: %#v", removed)
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
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "README.md"), "root\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "init")
	return repo
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestHasCommitsIgnoresCodexPromptOnlyCommit(t *testing.T) {
	repo := initRepo(t)
	m := New(repo, "", "main")
	wtPath, err := m.Create("sw-05", "feat/sw-05")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	writeFile(t, filepath.Join(wtPath, ".codex-prompt.md"), "prompt\n")
	runGit(t, wtPath, "add", ".codex-prompt.md")
	runGit(t, wtPath, "commit", "-m", "prompt only")

	has, sha, err := m.HasCommits("sw-05", "main")
	if err != nil {
		t.Fatalf("has commits: %v", err)
	}
	if has {
		t.Fatalf("expected codex-prompt-only commit to be ignored, got has=true sha=%q", sha)
	}
}

func TestHasCommitsDoesNotAutoRescueUncommittedPromptOnly(t *testing.T) {
	repo := initRepo(t)
	m := New(repo, "", "main")
	wtPath, err := m.Create("sw-06", "feat/sw-06")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	writeFile(t, filepath.Join(wtPath, ".codex-prompt.md"), "prompt\n")
	statusBefore := runGit(t, wtPath, "status", "--porcelain")
	if strings.TrimSpace(statusBefore) == "" {
		t.Fatalf("expected uncommitted prompt file before check")
	}

	has, sha, err := m.HasCommits("sw-06", "main")
	if err != nil {
		t.Fatalf("has commits: %v", err)
	}
	if has {
		t.Fatalf("expected no commits, got has=true sha=%q", sha)
	}

	statusAfter := runGit(t, wtPath, "status", "--porcelain")
	if strings.TrimSpace(statusAfter) == "" {
		t.Fatalf("expected uncommitted prompt file to remain (no auto-rescue commit)")
	}
}
