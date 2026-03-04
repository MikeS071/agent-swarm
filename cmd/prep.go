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

type prepIssue struct {
	Ticket string `json:"ticket,omitempty"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason"`
}

func runPrepChecks(cfg *config.Config, tr *tracker.Tracker, promptDir string) []prepIssue {
	issues := make([]prepIssue, 0)
	for id, tk := range tr.Tickets {
		if tk.Status == tracker.StatusDone {
			continue
		}
		if cfg.Project.RequireExplicitRole && strings.TrimSpace(tk.Profile) == "" {
			issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: "missing explicit role/profile"})
		}
		if cfg.Project.RequireVerifyCmd && strings.TrimSpace(tk.VerifyCmd) == "" && strings.TrimSpace(cfg.Integration.VerifyCmd) == "" {
			issues = append(issues, prepIssue{Ticket: id, Field: "verify_cmd", Reason: "missing verify_cmd and no integration.verify_cmd fallback"})
		}
		promptPath := filepath.Join(promptDir, id+".md")
		if _, err := os.Stat(promptPath); err != nil {
			if os.IsNotExist(err) {
				issues = append(issues, prepIssue{Ticket: id, Field: "prompt", Reason: "missing prompt file"})
			} else {
				issues = append(issues, prepIssue{Ticket: id, Field: "prompt", Reason: err.Error()})
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Ticket == issues[j].Ticket {
			return issues[i].Field < issues[j].Field
		}
		return issues[i].Ticket < issues[j].Ticket
	})
	return issues
}

var prepJSON bool

var prepCmd = &cobra.Command{
	Use:          "prep",
	Short:        "Run strict preflight checks before swarm execution",
	SilenceUsage: true,
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

		issues := runPrepChecks(cfg, tr, promptDir)
		if prepJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{"ok": len(issues) == 0, "issues": issues})
		}
		if len(issues) == 0 {
			if !prepJSON {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "prep ok")
			}
			return nil
		}
		if !prepJSON {
			for _, iss := range issues {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s [%s]: %s\n", iss.Ticket, iss.Field, iss.Reason)
			}
		}
		return fmt.Errorf("prep failed: %d issue(s)", len(issues))
	},
}

func init() {
	prepCmd.Flags().BoolVar(&prepJSON, "json", false, "output prep report as JSON")
	rootCmd.AddCommand(prepCmd)
}
