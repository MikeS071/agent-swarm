package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

type phase1Step struct {
	Name string
	Run  func() error
}

func runPhase1Flow(out io.Writer, steps []phase1Step) error {
	if out == nil {
		out = io.Discard
	}

	for _, step := range steps {
		fmt.Fprintf(out, "==> %s\n", step.Name)
		if step.Run == nil {
			return fmt.Errorf("phase1 pipeline failed at %s: step runner is nil", step.Name)
		}
		if err := step.Run(); err != nil {
			return fmt.Errorf("phase1 pipeline failed at %s: %w", step.Name, err)
		}
		fmt.Fprintf(out, "ok: %s\n", step.Name)
	}
	return nil
}

func lintTicketsPhase1(trackerPath string, strict bool) error {
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return err
	}
	if !strict {
		return nil
	}

	validStatus := map[string]struct{}{
		tracker.StatusTodo:    {},
		tracker.StatusRunning: {},
		tracker.StatusDone:    {},
		tracker.StatusFailed:  {},
		"blocked":            {},
	}

	ids := tr.DependencyOrder()
	if len(ids) == 0 {
		return errors.New("tracker has no tickets")
	}

	for _, id := range ids {
		tk := tr.Tickets[id]
		if strings.TrimSpace(tk.Desc) == "" {
			return fmt.Errorf("ticket %s missing desc", id)
		}
		if tk.Phase <= 0 {
			return fmt.Errorf("ticket %s has invalid phase %d", id, tk.Phase)
		}
		if _, ok := validStatus[tk.Status]; !ok {
			return fmt.Errorf("ticket %s has invalid status %q", id, tk.Status)
		}
	}
	return nil
}

func buildAllPromptsPhase1(trackerPath, promptDir string, strict bool) error {
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return err
	}

	ids := tr.DependencyOrder()
	for _, id := range ids {
		tk := tr.Tickets[id]
		if tk.Status != tracker.StatusTodo {
			continue
		}
		if strict && strings.TrimSpace(tk.Desc) == "" {
			return fmt.Errorf("ticket %s missing desc", id)
		}
		if _, err := GeneratePrompt(trackerPath, promptDir, id); err != nil {
			return fmt.Errorf("build %s: %w", id, err)
		}
	}
	return nil
}

func validatePromptsPhase1(trackerPath, promptDir string, strict bool) error {
	missing, err := CheckPrompts(trackerPath, promptDir)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing prompts: %s", strings.Join(missing, ", "))
	}
	if !strict {
		return nil
	}

	entries, err := os.ReadDir(promptDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("prompt directory %s does not exist", promptDir)
		}
		return err
	}

	issues := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(promptDir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, issue := range validatePromptFileContent(string(body)) {
			issues = append(issues, fmt.Sprintf("%s: %s", entry.Name(), issue))
		}
	}

	if len(issues) > 0 {
		sort.Strings(issues)
		return errors.New(strings.Join(issues, "; "))
	}
	return nil
}

func validatePromptFileContent(content string) []string {
	issues := make([]string, 0)
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []string{"prompt is empty"}
	}
	if !strings.HasPrefix(trimmed, "# ") {
		issues = append(issues, "missing top-level heading")
	}

	requiredSections := []string{"## Context", "## Dependencies", "## Your Scope", "## Verification"}
	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			issues = append(issues, "missing required section "+section)
		}
	}

	placeholderMarkers := []string{"TODO", "TBD", "<...>", "Add details here"}
	for _, marker := range placeholderMarkers {
		if strings.Contains(content, marker) {
			issues = append(issues, "contains placeholder marker "+marker)
		}
	}
	return issues
}

var prepStrict bool

var prepCmd = &cobra.Command{
	Use:          "prep",
	Short:        "Run phase1 prep checks (lint -> build -> validate)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		steps := []phase1Step{
			{Name: "tickets lint", Run: func() error { return lintTicketsPhase1(trackerPath, prepStrict) }},
			{Name: "prompts build --all", Run: func() error { return buildAllPromptsPhase1(trackerPath, promptDir, prepStrict) }},
			{Name: "prompts validate --strict", Run: func() error { return validatePromptsPhase1(trackerPath, promptDir, prepStrict) }},
		}

		if err := runPhase1Flow(cmd.OutOrStdout(), steps); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "phase1 pipeline passed")
		return err
	},
}

func init() {
	prepCmd.Flags().BoolVar(&prepStrict, "strict", false, "enforce strict ticket and prompt checks")
	rootCmd.AddCommand(prepCmd)
}
