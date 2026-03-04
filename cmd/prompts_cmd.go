package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	promptvalidate "github.com/MikeS071/agent-swarm/internal/prompts"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var promptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "Manage ticket prompt templates",
}

const promptRuleMissingFile = "prompt_file_missing"

type promptValidationFailure struct {
	Ticket  string `json:"ticket"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type promptTicketFailure struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type promptTicketResult struct {
	Ticket   string                `json:"ticket"`
	Valid    bool                  `json:"valid"`
	Failures []promptTicketFailure `json:"failures,omitempty"`
}

type promptValidationOutput struct {
	Valid    bool                      `json:"valid"`
	Strict   bool                      `json:"strict"`
	Ticket   string                    `json:"ticket,omitempty"`
	Results  []promptTicketResult      `json:"results"`
	Failures []promptValidationFailure `json:"failures,omitempty"`
}

var (
	promptsValidateStrict bool
	promptsValidateJSON   bool
	promptsValidateTicket string
)

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

var promptsGenCmd = &cobra.Command{
	Use:          "gen <ticket>",
	Short:        "Generate prompt template for a ticket",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		path, err := GeneratePrompt(trackerPath, promptDir, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
		return err
	},
}

var promptsValidateCmd = &cobra.Command{
	Use:          "validate",
	Short:        "Validate prompt sections and strict quality rules",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		promptDir := resolveFromConfig(cfgFile, cfg.Project.PromptDir)

		ticketIDs, err := promptValidationTicketIDs(trackerPath, strings.TrimSpace(promptsValidateTicket))
		if err != nil {
			return err
		}

		results := make([]promptTicketResult, 0, len(ticketIDs))
		flatFailures := make([]promptValidationFailure, 0)
		for _, ticketID := range ticketIDs {
			result := promptTicketResult{
				Ticket: ticketID,
				Valid:  true,
			}

			promptPath := filepath.Join(promptDir, ticketID+".md")
			body, readErr := os.ReadFile(promptPath)
			if readErr != nil {
				if os.IsNotExist(readErr) {
					result.Valid = false
					failure := promptTicketFailure{
						Rule:    promptRuleMissingFile,
						Message: "prompt file not found",
					}
					result.Failures = append(result.Failures, failure)
					flatFailures = append(flatFailures, promptValidationFailure{
						Ticket:  ticketID,
						Rule:    failure.Rule,
						Message: failure.Message,
					})
				} else {
					return fmt.Errorf("read prompt %s: %w", promptPath, readErr)
				}
			} else {
				report := promptvalidate.Validate(string(body), promptvalidate.Options{Strict: promptsValidateStrict})
				result.Valid = report.Valid
				for _, failure := range report.Failures {
					ticketFailure := promptTicketFailure{
						Rule:    failure.Rule,
						Message: failure.Message,
					}
					result.Failures = append(result.Failures, ticketFailure)
					flatFailures = append(flatFailures, promptValidationFailure{
						Ticket:  ticketID,
						Rule:    failure.Rule,
						Message: failure.Message,
					})
				}
			}

			results = append(results, result)
		}

		output := promptValidationOutput{
			Valid:    len(flatFailures) == 0,
			Strict:   promptsValidateStrict,
			Ticket:   strings.TrimSpace(promptsValidateTicket),
			Results:  results,
			Failures: flatFailures,
		}

		if promptsValidateJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(output); err != nil {
				return err
			}
		} else {
			if len(results) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no tickets selected for validation"); err != nil {
					return err
				}
			}
			for _, result := range results {
				status := "ok"
				if !result.Valid {
					status = "fail"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", result.Ticket, status); err != nil {
					return err
				}
				for _, failure := range result.Failures {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", failure.Rule, failure.Message); err != nil {
						return err
					}
				}
			}
		}

		if promptsValidateStrict && !output.Valid {
			return fmt.Errorf("prompt validation failed")
		}
		return nil
	},
}

func promptValidationTicketIDs(trackerPath, ticket string) ([]string, error) {
	tr, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}

	if ticket != "" {
		if _, ok := tr.Tickets[ticket]; !ok {
			return nil, fmt.Errorf("ticket %q not found", ticket)
		}
		return []string{ticket}, nil
	}

	ids := make([]string, 0)
	for ticketID, tk := range tr.Tickets {
		if tk.Status == tracker.StatusTodo {
			ids = append(ids, ticketID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func init() {
	promptsCmd.AddCommand(promptsCheckCmd)
	promptsCmd.AddCommand(promptsGenCmd)
	promptsValidateCmd.Flags().BoolVar(&promptsValidateStrict, "strict", false, "fail when validation rules are violated")
	promptsValidateCmd.Flags().BoolVar(&promptsValidateJSON, "json", false, "output results as JSON")
	promptsValidateCmd.Flags().StringVar(&promptsValidateTicket, "ticket", "", "validate a single ticket prompt")
	promptsCmd.AddCommand(promptsValidateCmd)
	rootCmd.AddCommand(promptsCmd)
}
