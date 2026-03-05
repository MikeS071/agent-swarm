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

type ticketsLintIssue struct {
	Ticket string `json:"ticket,omitempty"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason"`
}

var ticketsLintJSON bool

var ticketsLintCmd = &cobra.Command{
	Use:           "lint",
	Short:         "Lint tickets for schema/readiness quality",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
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

		issues := runTicketsLintChecks(cfg, tr, promptDir)
		if ticketsLintJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"ok":     len(issues) == 0,
				"issues": issues,
			})
		}
		if len(issues) == 0 {
			if !ticketsLintJSON {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "tickets lint ok")
			}
			return nil
		}

		if !ticketsLintJSON {
			for _, iss := range issues {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %s\n", iss.Ticket, iss.Field, iss.Reason)
			}
		}
		return fmt.Errorf("tickets lint failed: %d issue(s)", len(issues))
	},
}

func init() {
	ticketsLintCmd.Flags().BoolVar(&ticketsLintJSON, "json", false, "output report as JSON")
}

func runTicketsLintChecks(cfg *config.Config, tr *tracker.Tracker, promptDir string) []ticketsLintIssue {
	issues := make([]ticketsLintIssue, 0)

	// Include strict readiness checks used by prep gate.
	for _, iss := range runPrepChecks(cfg, tr, promptDir) {
		issues = append(issues, ticketsLintIssue{Ticket: iss.Ticket, Field: iss.Field, Reason: iss.Reason})
	}

	validStatuses := map[string]struct{}{
		tracker.StatusTodo:    {},
		tracker.StatusRunning: {},
		tracker.StatusDone:    {},
		tracker.StatusFailed:  {},
		"blocked":             {},
	}

	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		tk := tr.Tickets[id]
		if strings.TrimSpace(id) == "" {
			issues = append(issues, ticketsLintIssue{Field: "ticket", Reason: "empty ticket id"})
			continue
		}
		if tk.Phase <= 0 {
			issues = append(issues, ticketsLintIssue{Ticket: id, Field: "phase", Reason: "phase must be > 0"})
		}
		if _, ok := validStatuses[tk.Status]; !ok {
			issues = append(issues, ticketsLintIssue{Ticket: id, Field: "status", Reason: fmt.Sprintf("invalid status %q", tk.Status)})
		}
		if strings.TrimSpace(tk.Desc) == "" {
			issues = append(issues, ticketsLintIssue{Ticket: id, Field: "desc", Reason: "missing description"})
		}
		if strings.TrimSpace(tk.Branch) == "" {
			issues = append(issues, ticketsLintIssue{Ticket: id, Field: "branch", Reason: "missing branch"})
		}

		seenDeps := map[string]bool{}
		for _, dep := range tk.Depends {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				issues = append(issues, ticketsLintIssue{Ticket: id, Field: "depends", Reason: "empty dependency id"})
				continue
			}
			if dep == id {
				issues = append(issues, ticketsLintIssue{Ticket: id, Field: "depends", Reason: "self dependency is not allowed"})
			}
			if seenDeps[dep] {
				issues = append(issues, ticketsLintIssue{Ticket: id, Field: "depends", Reason: fmt.Sprintf("duplicate dependency %q", dep)})
			}
			seenDeps[dep] = true
			if _, ok := tr.Tickets[dep]; !ok {
				issues = append(issues, ticketsLintIssue{Ticket: id, Field: "depends", Reason: fmt.Sprintf("unknown dependency %q", dep)})
			}
		}

		if tk.Status == tracker.StatusDone {
			continue
		}
		promptPath := filepath.Join(promptDir, id+".md")
		promptBody, err := os.ReadFile(promptPath)
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(promptBody))
		for _, sec := range []string{"## objective", "## dependencies", "## scope", "## verify"} {
			if !strings.Contains(lower, sec) {
				issues = append(issues, ticketsLintIssue{Ticket: id, Field: "prompt", Reason: fmt.Sprintf("missing section %q", sec)})
			}
		}
	}

	return dedupeTicketsLintIssues(issues)
}

func dedupeTicketsLintIssues(in []ticketsLintIssue) []ticketsLintIssue {
	seen := map[string]bool{}
	out := make([]ticketsLintIssue, 0, len(in))
	for _, iss := range in {
		k := iss.Ticket + "|" + iss.Field + "|" + iss.Reason
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, iss)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ticket == out[j].Ticket {
			if out[i].Field == out[j].Field {
				return out[i].Reason < out[j].Reason
			}
			return out[i].Field < out[j].Field
		}
		return out[i].Ticket < out[j].Ticket
	})
	return out
}
