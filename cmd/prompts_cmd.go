package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	promptvalidator "github.com/MikeS071/agent-swarm/internal/prompts"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Manage ticket prompt templates",
}

var promptsCheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Report todo tickets missing prompt files",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		missing, err := CheckPrompts(trackerPath, promptDir)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "all todo tickets have prompts")
			return err
		}
		for _, ticketID := range missing {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), ticketID); err != nil {
				return err
			}
		}
		return fmt.Errorf("missing prompts: %s", strings.Join(missing, ", "))
	},
}

var (
	promptsValidateStrict bool
	promptsValidateJSON   bool
	promptsValidateTicket string
)

var promptsValidateCmd = &cobra.Command{
	Use:           "validate",
	Short:         "Validate ticket prompt quality",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		issues, err := validatePrompts(trackerPath, promptDir, strings.TrimSpace(promptsValidateTicket), promptsValidateStrict)
		if err != nil {
			return err
		}

		if promptsValidateJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if encodeErr := enc.Encode(map[string]any{"ok": len(issues) == 0, "issues": issues}); encodeErr != nil {
				return encodeErr
			}
		}
		if len(issues) == 0 {
			if !promptsValidateJSON {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "prompts validation ok")
				return err
			}
			return nil
		}

		if !promptsValidateJSON {
			for _, issue := range issues {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %s\n", issue.Ticket, issue.Rule, issue.Message); err != nil {
					return err
				}
			}
		}
		return fmt.Errorf("prompts validation failed: %d issue(s)", len(issues))
	},
}

func validatePrompts(trackerPath, promptDir, ticketFilter string, strict bool) ([]promptvalidator.Issue, error) {
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}

	ticketIDs := collectPromptTicketIDs(tr, ticketFilter)
	if len(ticketIDs) == 0 {
		if ticketFilter != "" {
			return nil, fmt.Errorf("ticket %q not found", ticketFilter)
		}
		return nil, nil
	}

	issues := make([]promptvalidator.Issue, 0)
	for _, ticketID := range ticketIDs {
		promptPath := filepath.Join(promptDir, ticketID+".md")
		body, readErr := os.ReadFile(promptPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				issues = append(issues, promptvalidator.Issue{
					Ticket:  ticketID,
					Rule:    "prompt_file_exists",
					Message: "missing prompt file",
				})
				continue
			}
			return nil, readErr
		}
		issues = append(issues, promptvalidator.ValidatePrompt(ticketID, body, strict)...)
	}
	return issues, nil
}

func collectPromptTicketIDs(tr *tracker.Tracker, ticketFilter string) []string {
	if tr == nil {
		return nil
	}
	if ticketFilter != "" {
		if _, ok := tr.Tickets[ticketFilter]; !ok {
			return nil
		}
		return []string{ticketFilter}
	}
	ids := make([]string, 0, len(tr.Tickets))
	for ticketID, tk := range tr.Tickets {
		if tk.Status == tracker.StatusDone {
			continue
		}
		ids = append(ids, ticketID)
	}
	sort.Strings(ids)
	return ids
}

func init() {
	promptsCmd.AddCommand(promptsCheckCmd)
	promptsValidateCmd.Flags().BoolVar(&promptsValidateStrict, "strict", false, "enable strict prompt validation checks")
	promptsValidateCmd.Flags().BoolVar(&promptsValidateJSON, "json", false, "output validation results as JSON")
	promptsValidateCmd.Flags().StringVar(&promptsValidateTicket, "ticket", "", "validate only the specified ticket")
	promptsCmd.AddCommand(promptsValidateCmd)
	rootCmd.AddCommand(promptsCmd)
}
