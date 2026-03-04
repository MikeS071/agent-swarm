package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AgentContextOptions configures immutable context snapshot materialization.
type AgentContextOptions struct {
	TicketID     string
	WorktreePath string
	Role         string
	RunID        string
	CreatedAt    time.Time
}

// ContextManifest is the auditable snapshot index stored in the worktree.
type ContextManifest struct {
	Ticket    string                  `json:"ticket"`
	RunID     string                  `json:"runId,omitempty"`
	Role      string                  `json:"role,omitempty"`
	CreatedAt string                  `json:"createdAt"`
	Sources   []ContextManifestSource `json:"sources"`
}

// ContextManifestSource records one source file hash in the manifest.
type ContextManifestSource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// MaterializeAgentContext creates a deterministic immutable snapshot at
// <worktree>/.agent-context and writes context-manifest.json.
func (m *Manager) MaterializeAgentContext(opts AgentContextOptions) (string, ContextManifest, error) {
	if m == nil {
		return "", ContextManifest{}, errors.New("worktree manager is nil")
	}
	ticketID := strings.TrimSpace(opts.TicketID)
	if ticketID == "" {
		return "", ContextManifest{}, errors.New("ticketID is required")
	}
	worktreePath := strings.TrimSpace(opts.WorktreePath)
	if worktreePath == "" {
		return "", ContextManifest{}, errors.New("worktree path is required")
	}
	projectRoot := strings.TrimSpace(m.RepoDir)
	if projectRoot == "" {
		return "", ContextManifest{}, errors.New("repo dir is required")
	}

	role := strings.TrimSpace(opts.Role)
	createdAt := opts.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}

	sources, err := m.resolveContextSources(projectRoot, role)
	if err != nil {
		return "", ContextManifest{}, err
	}

	contextRoot := filepath.Join(worktreePath, ".agent-context")
	if err := os.RemoveAll(contextRoot); err != nil {
		return "", ContextManifest{}, fmt.Errorf("reset context dir %s: %w", contextRoot, err)
	}
	if err := os.MkdirAll(contextRoot, 0o755); err != nil {
		return "", ContextManifest{}, fmt.Errorf("mkdir context dir %s: %w", contextRoot, err)
	}

	manifest := ContextManifest{
		Ticket:    ticketID,
		RunID:     strings.TrimSpace(opts.RunID),
		Role:      role,
		CreatedAt: createdAt.Format(time.RFC3339),
		Sources:   make([]ContextManifestSource, 0, len(sources)),
	}

	for _, rel := range sources {
		srcPath := filepath.Join(projectRoot, filepath.FromSlash(rel))
		content, err := os.ReadFile(srcPath)
		if err != nil {
			return "", ContextManifest{}, fmt.Errorf("read context source %s: %w", srcPath, err)
		}
		sum := sha256.Sum256(content)
		manifest.Sources = append(manifest.Sources, ContextManifestSource{
			Path:   rel,
			SHA256: hex.EncodeToString(sum[:]),
		})

		destRel, err := resolveSnapshotDestination(rel)
		if err != nil {
			return "", ContextManifest{}, err
		}
		destPath := filepath.Join(contextRoot, filepath.FromSlash(destRel))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", ContextManifest{}, fmt.Errorf("mkdir context snapshot dir for %s: %w", destPath, err)
		}
		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			return "", ContextManifest{}, fmt.Errorf("write context snapshot %s: %w", destPath, err)
		}
	}

	manifestPath := filepath.Join(contextRoot, "context-manifest.json")
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", ContextManifest{}, fmt.Errorf("marshal context manifest: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		return "", ContextManifest{}, fmt.Errorf("write context manifest %s: %w", manifestPath, err)
	}

	return manifestPath, manifest, nil
}

func (m *Manager) resolveContextSources(projectRoot, role string) ([]string, error) {
	seen := map[string]struct{}{}

	add := func(rel string) {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if rel == "" {
			return
		}
		seen[rel] = struct{}{}
	}

	rulesDir := filepath.Join(projectRoot, ".codex", "rules")
	rules, err := collectRelativeFiles(projectRoot, rulesDir, func(rel string, d fs.DirEntry) bool {
		return strings.HasSuffix(strings.ToLower(rel), ".md")
	})
	if err != nil {
		return nil, fmt.Errorf("collect rules sources: %w", err)
	}
	for _, rel := range rules {
		add(rel)
	}

	if role != "" {
		profileRel := filepath.ToSlash(filepath.Join(".agents", "profiles", role+".md"))
		profilePath := filepath.Join(projectRoot, filepath.FromSlash(profileRel))
		if _, err := os.Stat(profilePath); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("profile snapshot source not found for role %q at %s", role, profilePath)
			}
			return nil, fmt.Errorf("stat profile snapshot source %s: %w", profilePath, err)
		}
		add(profileRel)

		roleRel := filepath.ToSlash(filepath.Join(".agents", "roles", role+".yaml"))
		rolePath := filepath.Join(projectRoot, filepath.FromSlash(roleRel))
		if _, err := os.Stat(rolePath); err == nil {
			add(roleRel)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat role snapshot source %s: %w", rolePath, err)
		}
	}

	skillsDir := filepath.Join(projectRoot, ".agents", "skills")
	skills, err := collectRelativeFiles(projectRoot, skillsDir, func(_ string, d fs.DirEntry) bool {
		return strings.EqualFold(d.Name(), "SKILL.md")
	})
	if err != nil {
		return nil, fmt.Errorf("collect skills sources: %w", err)
	}
	for _, rel := range skills {
		add(rel)
	}

	out := make([]string, 0, len(seen))
	for rel := range seen {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

func collectRelativeFiles(projectRoot, dir string, include func(rel string, d fs.DirEntry) bool) ([]string, error) {
	entries := make([]string, 0)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return entries, nil
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		if include == nil || include(rel, d) {
			entries = append(entries, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	return entries, nil
}

func resolveSnapshotDestination(sourceRel string) (string, error) {
	switch {
	case strings.HasPrefix(sourceRel, ".codex/rules/"):
		return "rules/" + strings.TrimPrefix(sourceRel, ".codex/rules/"), nil
	case strings.HasPrefix(sourceRel, ".agents/profiles/"):
		return "profiles/" + strings.TrimPrefix(sourceRel, ".agents/profiles/"), nil
	case strings.HasPrefix(sourceRel, ".agents/roles/"):
		return "roles/" + strings.TrimPrefix(sourceRel, ".agents/roles/"), nil
	case strings.HasPrefix(sourceRel, ".agents/skills/"):
		return "skills/" + strings.TrimPrefix(sourceRel, ".agents/skills/"), nil
	default:
		return "", fmt.Errorf("unsupported context source path %q", sourceRel)
	}
}
