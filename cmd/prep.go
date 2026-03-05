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
	projectRoot := strings.TrimSpace(cfg.Project.Repo)

	for id, tk := range tr.Tickets {
		if tk.Status == tracker.StatusDone {
			continue
		}

		profile := strings.TrimSpace(tk.Profile)
		ticketType := strings.TrimSpace(tk.Type)

		if cfg.Project.RequireExplicitRole && profile == "" {
			issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: "missing explicit role/profile"})
		}
		if cfg.Project.RequireVerifyCmd && strings.TrimSpace(tk.VerifyCmd) == "" && strings.TrimSpace(cfg.Integration.VerifyCmd) == "" {
			issues = append(issues, prepIssue{Ticket: id, Field: "verify_cmd", Reason: "missing verify_cmd and no integration.verify_cmd fallback"})
		}

		promptPath := filepath.Join(promptDir, id+".md")
		promptBody, promptErr := os.ReadFile(promptPath)
		if promptErr != nil {
			if os.IsNotExist(promptErr) {
				issues = append(issues, prepIssue{Ticket: id, Field: "prompt", Reason: "missing prompt file"})
			} else {
				issues = append(issues, prepIssue{Ticket: id, Field: "prompt", Reason: promptErr.Error()})
			}
		}

		if profile != "" {
			profilePath := filepath.Join(projectRoot, ".agents", "profiles", profile+".md")
			if profileBody, err := os.ReadFile(profilePath); err != nil {
				issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: fmt.Sprintf("profile file missing: %s", profilePath)})
			} else {
				meta := parseProfileFrontmatter(profileBody)
				if meta["name"] == "" {
					issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: "profile frontmatter missing name"})
				} else if meta["name"] != profile {
					issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: fmt.Sprintf("profile frontmatter name mismatch: got %q want %q", meta["name"], profile)})
				}
				if meta["description"] == "" {
					issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: "profile frontmatter missing description"})
				}
				if meta["mode"] == "" {
					issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: "profile frontmatter missing mode"})
				}
			}
		}

		if expected := expectedProfileForType(ticketType); expected != "" && profile != "" && profile != expected {
			issues = append(issues, prepIssue{Ticket: id, Field: "profile", Reason: fmt.Sprintf("ticket type %q should use profile %q (got %q)", ticketType, expected, profile)})
		}

		if len(promptBody) > 0 {
			for _, req := range requiredPromptSnippets(ticketType) {
				if !strings.Contains(strings.ToLower(string(promptBody)), strings.ToLower(req)) {
					issues = append(issues, prepIssue{Ticket: id, Field: "prompt", Reason: fmt.Sprintf("ticket type %q prompt missing required snippet: %s", ticketType, req)})
				}
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

func expectedProfileForType(ticketType string) string {
	switch strings.TrimSpace(ticketType) {
	case "int":
		return "code-agent"
	case "gap", "review":
		return "code-reviewer"
	case "tst":
		return "e2e-runner"
	case "sec":
		return "security-reviewer"
	case "doc", "mem":
		return "doc-updater"
	case "clean":
		return "refactor-cleaner"
	default:
		return ""
	}
}

func requiredPromptSnippets(ticketType string) []string {
	base := []string{"## objective", "## dependencies", "## scope", "## verify"}
	switch strings.TrimSpace(ticketType) {
	case "gap":
		return append(base, "gap-report.md")
	case "tst":
		return append(base, "test-report.md")
	case "review":
		return append(base, "review-report.json", "read-only")
	case "sec":
		return append(base, "sec-report.json", "read-only")
	case "doc":
		return append(base, "docs/user-guide.md", "docs/technical.md", "docs/release-notes.md", "doc-report.md")
	case "clean":
		return append(base, "clean-report.md")
	case "mem":
		return append(base, "docs/lessons-learned.md", "mem-report.md", "decisions", "lessons")
	default:
		return nil
	}
}

func parseProfileFrontmatter(data []byte) map[string]string {
	out := map[string]string{}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		out[k] = v
	}
	return out
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
