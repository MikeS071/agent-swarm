package worktree

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Manager struct {
	RepoDir      string
	WorktreeBase string
	BaseBranch   string
}

type Worktree struct {
	Path   string
	Head   string
	Branch string
}

func New(repoDir, worktreeBase, baseBranch string) *Manager {
	if baseBranch == "" {
		baseBranch = "main"
	}
	if worktreeBase == "" {
		repoBase := filepath.Base(filepath.Clean(repoDir))
		worktreeBase = filepath.Join(filepath.Dir(filepath.Clean(repoDir)), repoBase+"-worktrees")
	}
	return &Manager{
		RepoDir:      repoDir,
		WorktreeBase: worktreeBase,
		BaseBranch:   baseBranch,
	}
}

func (m *Manager) Path(ticketID string) string {
	return filepath.Join(m.WorktreeBase, ticketID)
}

func (m *Manager) Exists(ticketID string) bool {
	_, err := os.Stat(m.Path(ticketID))
	return err == nil
}

func (m *Manager) Create(ticketID, branch string) (string, error) {
	if ticketID == "" {
		return "", errors.New("ticketID is required")
	}
	if branch == "" {
		return "", errors.New("branch is required")
	}
	if err := os.MkdirAll(m.WorktreeBase, 0o755); err != nil {
		return "", err
	}
	path := m.Path(ticketID)
	if _, err := m.git(m.RepoDir, "worktree", "add", "-b", branch, path, m.BaseBranch); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) Remove(ticketID string) error {
	path := m.Path(ticketID)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := m.git(m.RepoDir, "worktree", "remove", "--force", path); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Prune() error {
	_, err := m.git(m.RepoDir, "worktree", "prune")
	return err
}

func (m *Manager) List() ([]Worktree, error) {
	out, err := m.git(m.RepoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelainList(out), nil
}

func (m *Manager) HasCommits(ticketID, baseBranch string) (bool, string, error) {
	if baseBranch == "" {
		baseBranch = m.BaseBranch
	}
	branch := "feat/" + ticketID
	wtPath := m.Path(ticketID)

	// If worktree directory doesn't exist, treat as no commits
	if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
		return false, "", nil
	}

	logOut, err := m.git(wtPath, "log", fmt.Sprintf("%s..%s", baseBranch, branch), "--oneline")
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(logOut) == "" {
		return false, "", nil
	}

	sha, err := m.git(wtPath, "rev-parse", "--short", "HEAD")
	if err != nil {
		return false, "", err
	}
	return true, strings.TrimSpace(sha), nil
}

func (m *Manager) CleanupOlderThan(duration time.Duration) ([]string, error) {
	entries, err := os.ReadDir(m.WorktreeBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	cutoff := time.Now().Add(-duration)
	removed := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ticketID := entry.Name()
		path := m.Path(ticketID)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := m.Remove(ticketID); err != nil {
				return removed, err
			}
			removed = append(removed, ticketID)
		}
	}
	sort.Strings(removed)
	return removed, nil
}

func parsePorcelainList(out string) []Worktree {
	scanner := bufio.NewScanner(bytes.NewBufferString(out))
	worktrees := make([]Worktree, 0)
	current := Worktree{}
	seen := false

	flush := func() {
		if seen {
			worktrees = append(worktrees, current)
		}
		current = Worktree{}
		seen = false
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := parts[1]
		seen = true
		switch key {
		case "worktree":
			current.Path = val
		case "HEAD":
			current.Head = val
		case "branch":
			current.Branch = strings.TrimPrefix(val, "refs/heads/")
		}
	}
	flush()
	return worktrees
}

func (m *Manager) git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w (%s)", args, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
