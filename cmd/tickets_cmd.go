package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

const exitCodeValidation = 2

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	return e.err
}

func (e *exitCodeError) ExitCode() int {
	return e.code
}

func validationFailureError(format string, args ...any) error {
	return &exitCodeError{code: exitCodeValidation, err: fmt.Errorf(format, args...)}
}

var ticketsCmd = &cobra.Command{
	Use:   "tickets",
	Short: "Manage ticket quality checks",
}

var ticketsLintCmd = &cobra.Command{
	Use:          "lint",
	Short:        "Validate tracker tickets for schema completeness",
	SilenceUsage: true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		strict, err := cmd.Flags().GetBool("strict")
		if err != nil {
			return err
		}
		asJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			return err
		}

		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		if _, err := loadTrackerWithFallback(cfg, trackerPath); err != nil {
			return err
		}

		report, err := tracker.LintTickets(trackerPath, strict)
		if err != nil {
			return err
		}

		if asJSON {
			if err := writeTicketLintJSON(cmd.OutOrStdout(), report); err != nil {
				return err
			}
		} else {
			if report.IssueCount == 0 {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "tickets lint passed (%d tickets checked)\n", report.TicketCount); err != nil {
					return err
				}
			} else {
				if err := writeTicketLintText(cmd.OutOrStdout(), report); err != nil {
					return err
				}
			}
		}

		if report.IssueCount > 0 {
			return validationFailureError("ticket validation failed with %d issue(s)", report.IssueCount)
		}
		return nil
	},
}

func init() {
	ticketsLintCmd.Flags().Bool("strict", false, "fail on missing required schema v2 fields")
	ticketsLintCmd.Flags().Bool("json", false, "output JSON report")
	ticketsCmd.AddCommand(ticketsLintCmd)
	rootCmd.AddCommand(ticketsCmd)
}

func writeTicketLintJSON(out io.Writer, report tracker.TicketLintReport) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeTicketLintText(out io.Writer, report tracker.TicketLintReport) error {
	issuesByTicket := make(map[string][]tracker.TicketValidationIssue)
	keys := make([]string, 0)
	for _, issue := range report.Issues {
		ticketID := issue.TicketID
		if strings.TrimSpace(ticketID) == "" {
			ticketID = "(tracker)"
		}
		if _, ok := issuesByTicket[ticketID]; !ok {
			keys = append(keys, ticketID)
		}
		issuesByTicket[ticketID] = append(issuesByTicket[ticketID], issue)
	}
	sort.Strings(keys)

	for i, ticketID := range keys {
		if _, err := fmt.Fprintf(out, "%s:\n", ticketID); err != nil {
			return err
		}
		issues := issuesByTicket[ticketID]
		sort.Slice(issues, func(i, j int) bool {
			if issues[i].Path == issues[j].Path {
				return issues[i].Message < issues[j].Message
			}
			return issues[i].Path < issues[j].Path
		})
		for _, issue := range issues {
			if _, err := fmt.Fprintf(out, "  - %s: %s\n", issue.Path, issue.Message); err != nil {
				return err
			}
		}
		if i < len(keys)-1 {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	return nil
}

func validationExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}
