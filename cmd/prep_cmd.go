package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

const (
	prepStepTicketsLint     = "tickets lint"
	prepStepPromptsBuild    = "prompts build --all"
	prepStepPromptsValidate = "prompts validate --strict"
	prepStepSpawnability    = "spawnability checks"
)

type prepPipelineIssue struct {
	Step   string `json:"step"`
	Ticket string `json:"ticket,omitempty"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason"`
}

func init() {
	prepCmd.SilenceErrors = true
	prepCmd.RunE = runPrepCommand
}

func runPrepCommand(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
	cfg.Project.Tracker = trackerPath
	promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)
	cfg.Project.PromptDir = promptDir

	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return err
	}

	issues := runPrepPipeline(cfg, tr, promptDir)
	if prepJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"ok": len(issues) == 0, "issues": issues}); err != nil {
			return err
		}
		if len(issues) > 0 {
			return fmt.Errorf("prep failed: %d issue(s)", len(issues))
		}
		return nil
	}

	if err := renderPrepPipelineReport(cmd, issues); err != nil {
		return err
	}
	if len(issues) > 0 {
		return fmt.Errorf("prep failed: %d issue(s)", len(issues))
	}
	return nil
}

func runPrepPipeline(cfg *config.Config, tr *tracker.Tracker, promptDir string) []prepPipelineIssue {
	issues := make([]prepPipelineIssue, 0)
	issues = append(issues, runTicketsLintChecks(tr)...)
	issues = append(issues, runPromptsBuildChecks(tr, promptDir)...)
	issues = append(issues, runPromptsValidateStrictChecks(tr, promptDir)...)

	spawnabilityIssues := runPrepChecks(cfg, tr, promptDir)
	for _, issue := range spawnabilityIssues {
		issues = append(issues, prepPipelineIssue{
			Step:   prepStepSpawnability,
			Ticket: issue.Ticket,
			Field:  issue.Field,
			Reason: issue.Reason,
		})
	}

	sortPrepPipelineIssues(issues)
	return issues
}

func runTicketsLintChecks(tr *tracker.Tracker) []prepPipelineIssue {
	issues := make([]prepPipelineIssue, 0)
	if tr == nil {
		return []prepPipelineIssue{{Step: prepStepTicketsLint, Reason: "tracker is required"}}
	}

	validStatuses := map[string]struct{}{
		"todo":    {},
		"running": {},
		"done":    {},
		"failed":  {},
		"blocked": {},
	}

	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		tk := tr.Tickets[id]
		if tk.Phase <= 0 {
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepTicketsLint,
				Ticket: id,
				Field:  "phase",
				Reason: "phase must be > 0",
			})
		}
		status := strings.TrimSpace(tk.Status)
		if _, ok := validStatuses[status]; !ok {
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepTicketsLint,
				Ticket: id,
				Field:  "status",
				Reason: fmt.Sprintf("invalid status %q", tk.Status),
			})
		}
		for _, dep := range tk.Depends {
			dep = strings.TrimSpace(dep)
			switch {
			case dep == "":
				issues = append(issues, prepPipelineIssue{
					Step:   prepStepTicketsLint,
					Ticket: id,
					Field:  "depends",
					Reason: "contains empty dependency",
				})
			case dep == id:
				issues = append(issues, prepPipelineIssue{
					Step:   prepStepTicketsLint,
					Ticket: id,
					Field:  "depends",
					Reason: "self dependency is not allowed",
				})
			default:
				if _, ok := tr.Tickets[dep]; !ok {
					issues = append(issues, prepPipelineIssue{
						Step:   prepStepTicketsLint,
						Ticket: id,
						Field:  "depends",
						Reason: fmt.Sprintf("unknown dependency %q", dep),
					})
				}
			}
		}
	}

	return issues
}

func runPromptsBuildChecks(tr *tracker.Tracker, promptDir string) []prepPipelineIssue {
	issues := make([]prepPipelineIssue, 0)
	if strings.TrimSpace(promptDir) == "" {
		return []prepPipelineIssue{{
			Step:   prepStepPromptsBuild,
			Field:  "prompt_dir",
			Reason: "prompt directory is required",
		}}
	}

	info, err := os.Stat(promptDir)
	if err != nil {
		return []prepPipelineIssue{{
			Step:   prepStepPromptsBuild,
			Field:  "prompt_dir",
			Reason: fmt.Sprintf("prompt directory not readable: %v", err),
		}}
	}
	if !info.IsDir() {
		return []prepPipelineIssue{{
			Step:   prepStepPromptsBuild,
			Field:  "prompt_dir",
			Reason: "prompt_dir is not a directory",
		}}
	}

	ids := sortedActiveTicketIDs(tr)
	for _, id := range ids {
		promptPath := filepath.Join(promptDir, id+".md")
		if _, err := os.Stat(promptPath); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, prepPipelineIssue{
					Step:   prepStepPromptsBuild,
					Ticket: id,
					Field:  "prompt",
					Reason: "missing prompt file; run `agent-swarm prompts build --all`",
				})
				continue
			}
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepPromptsBuild,
				Ticket: id,
				Field:  "prompt",
				Reason: fmt.Sprintf("prompt file check failed: %v", err),
			})
		}
	}
	return issues
}

func runPromptsValidateStrictChecks(tr *tracker.Tracker, promptDir string) []prepPipelineIssue {
	issues := make([]prepPipelineIssue, 0)
	if tr == nil {
		return []prepPipelineIssue{{Step: prepStepPromptsValidate, Reason: "tracker is required"}}
	}

	ids := sortedActiveTicketIDs(tr)
	for _, id := range ids {
		promptPath := filepath.Join(promptDir, id+".md")
		b, err := os.ReadFile(promptPath)
		if err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, prepPipelineIssue{
					Step:   prepStepPromptsValidate,
					Ticket: id,
					Field:  "prompt",
					Reason: "prompt file missing; run `agent-swarm prompts build --all`",
				})
				continue
			}
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepPromptsValidate,
				Ticket: id,
				Field:  "prompt",
				Reason: fmt.Sprintf("prompt file read failed: %v", err),
			})
			continue
		}

		text := string(b)
		if strings.TrimSpace(text) == "" {
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepPromptsValidate,
				Ticket: id,
				Field:  "prompt",
				Reason: "prompt file is empty",
			})
			continue
		}

		for _, marker := range unresolvedPromptMarkers() {
			if strings.Contains(text, marker) {
				issues = append(issues, prepPipelineIssue{
					Step:   prepStepPromptsValidate,
					Ticket: id,
					Field:  "prompt",
					Reason: fmt.Sprintf("prompt contains unresolved placeholder %q", marker),
				})
			}
		}

		lower := strings.ToLower(text)
		if strings.Contains(lower, "tbd") {
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepPromptsValidate,
				Ticket: id,
				Field:  "prompt",
				Reason: "prompt contains unresolved placeholder \"TBD\"",
			})
		}
		if strings.Contains(lower, "add details here") {
			issues = append(issues, prepPipelineIssue{
				Step:   prepStepPromptsValidate,
				Ticket: id,
				Field:  "prompt",
				Reason: "prompt contains unresolved placeholder \"Add details here\"",
			})
		}
	}

	return issues
}

func unresolvedPromptMarkers() []string {
	return []string{"TODO", "<...>"}
}

func sortedActiveTicketIDs(tr *tracker.Tracker) []string {
	if tr == nil {
		return nil
	}
	ids := make([]string, 0, len(tr.Tickets))
	for id, tk := range tr.Tickets {
		if tk.Status == tracker.StatusDone {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortPrepPipelineIssues(issues []prepPipelineIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Step != issues[j].Step {
			return issues[i].Step < issues[j].Step
		}
		if issues[i].Ticket != issues[j].Ticket {
			return issues[i].Ticket < issues[j].Ticket
		}
		if issues[i].Field != issues[j].Field {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Reason < issues[j].Reason
	})
}

func renderPrepPipelineReport(cmd *cobra.Command, issues []prepPipelineIssue) error {
	order := []string{prepStepTicketsLint, prepStepPromptsBuild, prepStepPromptsValidate, prepStepSpawnability}
	failures := map[string]int{}
	for _, issue := range issues {
		failures[issue.Step]++
	}

	for _, step := range order {
		if failures[step] == 0 {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", step); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: fail (%d)\n", step, failures[step]); err != nil {
			return err
		}
	}

	if len(issues) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "prep ok")
		return err
	}

	for _, issue := range issues {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s [%s]: %s\n", issue.Step, issue.Ticket, issue.Field, issue.Reason); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), "run `agent-swarm prep --json` for machine-readable output")
	return err
}
