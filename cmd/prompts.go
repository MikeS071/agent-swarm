package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	intprompts "github.com/MikeS071/agent-swarm/internal/prompts"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func CheckPrompts(trackerPath, promptDir string) ([]string, error) {
	t, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}
	missing := make([]string, 0)
	for ticketID, ticket := range t.Tickets {
		if ticket.Status != "todo" {
			continue
		}
		promptPath := filepath.Join(promptDir, ticketID+".md")
		if _, err := os.Stat(promptPath); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, ticketID)
				continue
			}
			return nil, err
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func GeneratePrompt(trackerPath, promptDir, ticketID string) (string, error) {
	t, err := tracker.Load(trackerPath)
	if err != nil {
		return "", err
	}
	ticket, ok := t.Tickets[ticketID]
	if !ok {
		return "", errors.New("ticket not found")
	}
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return "", err
	}
	deps := "none"
	if len(ticket.Depends) > 0 {
		deps = strings.Join(ticket.Depends, ", ")
	}
	body := fmt.Sprintf(`# %s: %s

## Context
You are building a Go CLI called %q. Read SPEC.md for full spec.

## Dependencies
%s

## Your Scope
- Implement the ticket requirements
- Add or update tests first
- Ensure build and tests pass

## Notes
- Add implementation details here
- Add edge cases here
`, strings.ToUpper(ticketID), ticket.Desc, t.Project, deps)

	path := filepath.Join(promptDir, ticketID+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

type PromptBuildArtifact struct {
	TicketID     string
	PromptPath   string
	ManifestPath string
}

func BuildPrompts(trackerPath, promptDir, ticketID string, buildAll bool, strict bool, policy intprompts.PolicyContext) ([]PromptBuildArtifact, error) {
	t, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}
	ids, err := selectBuildTicketIDs(t, ticketID, buildAll)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return nil, err
	}

	type compiledOutput struct {
		ticketID string
		compiled intprompts.CompiledArtifact
	}
	compiled := make([]compiledOutput, 0, len(ids))
	for _, id := range ids {
		artifact, err := intprompts.Compile(id, t.Tickets[id], policy, intprompts.CompileOptions{
			Strict: strict,
		})
		if err != nil {
			return nil, fmt.Errorf("build prompt for %s: %w", id, err)
		}
		compiled = append(compiled, compiledOutput{
			ticketID: id,
			compiled: artifact,
		})
	}

	built := make([]PromptBuildArtifact, 0, len(ids))
	for _, output := range compiled {
		promptPath := filepath.Join(promptDir, output.ticketID+".md")
		if err := os.WriteFile(promptPath, output.compiled.Prompt, 0o644); err != nil {
			return nil, fmt.Errorf("write prompt %s: %w", promptPath, err)
		}
		manifestPath := filepath.Join(promptDir, output.ticketID+".manifest.json")
		if err := os.WriteFile(manifestPath, output.compiled.Manifest, 0o644); err != nil {
			return nil, fmt.Errorf("write manifest %s: %w", manifestPath, err)
		}
		built = append(built, PromptBuildArtifact{
			TicketID:     output.ticketID,
			PromptPath:   promptPath,
			ManifestPath: manifestPath,
		})
	}
	return built, nil
}

func BuildPromptPolicyContext(cfg *config.Config, configPath string) intprompts.PolicyContext {
	if cfg == nil {
		return intprompts.PolicyContext{}
	}
	repo := strings.TrimSpace(cfg.Project.Repo)
	if repo == "" {
		repo = filepath.Dir(configPath)
	}

	pointers := make([]string, 0, 6)
	addPointer := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo, path)
		}
		if _, err := os.Stat(path); err != nil {
			return
		}
		if rel, err := filepath.Rel(repo, path); err == nil && !strings.HasPrefix(rel, "..") {
			pointers = append(pointers, filepath.ToSlash(filepath.Clean(rel)))
			return
		}
		pointers = append(pointers, filepath.ToSlash(filepath.Clean(path)))
	}

	addPointer(configPath)
	addPointer(cfg.Project.SpecFile)
	addPointer(filepath.Join(repo, ".agents", "AGENTS.md"))
	addPointer(filepath.Join(repo, "swarm", "prompt-footer.md"))

	if entries, err := os.ReadDir(filepath.Join(repo, ".codex", "rules", "common")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			addPointer(filepath.Join(repo, ".codex", "rules", "common", entry.Name()))
		}
	}
	sort.Strings(pointers)

	spec := strings.TrimSpace(cfg.Project.SpecFile)
	if rel, err := filepath.Rel(repo, spec); err == nil && !strings.HasPrefix(rel, "..") {
		spec = filepath.ToSlash(filepath.Clean(rel))
	}

	return intprompts.PolicyContext{
		ProjectName:          strings.TrimSpace(cfg.Project.Name),
		BaseBranch:           strings.TrimSpace(cfg.Project.BaseBranch),
		SpecFile:             spec,
		DefaultVerifyCommand: strings.TrimSpace(cfg.Integration.VerifyCmd),
		AgentContextPointers: pointers,
	}
}

func selectBuildTicketIDs(t *tracker.Tracker, ticketID string, buildAll bool) ([]string, error) {
	if t == nil {
		return nil, errors.New("tracker is nil")
	}
	if buildAll {
		ids := make([]string, 0, len(t.Tickets))
		for id := range t.Tickets {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids, nil
	}

	id := strings.TrimSpace(ticketID)
	if id == "" {
		return nil, errors.New("ticket id is required")
	}
	if _, ok := t.Tickets[id]; !ok {
		return nil, errors.New("ticket not found")
	}
	return []string{id}, nil
}
