package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	guardianevidence "github.com/MikeS071/agent-swarm/internal/guardian/evidence"
	"github.com/spf13/cobra"
)

var guardianReportJSON bool

var guardianCmd = &cobra.Command{
	Use:   "guardian",
	Short: "Guardian governance and evidence utilities",
}

var guardianReportCmd = &cobra.Command{
	Use:          "report",
	Short:        "Show guardian decision report from evidence store",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		stateRoot := filepath.Dir(trackerPath)
		evidenceDir := filepath.Join(stateRoot, "guardian", "evidence")

		entries, err := loadGuardianEvidence(evidenceDir)
		if err != nil {
			return err
		}
		payload := buildGuardianReportPayload(entries)
		if guardianReportJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "guardian evidence: total=%d block=%d warn=%d allow=%d\n",
			payload.Total, payload.Counts.Block, payload.Counts.Warn, payload.Counts.Allow)
		for _, e := range payload.Recent {
			ticket := strings.TrimSpace(e.TicketID)
			if ticket == "" {
				ticket = "-"
			}
			rule := strings.TrimSpace(e.RuleID)
			if rule == "" {
				rule = "-"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s ticket=%s rule=%s reason=%s\n", e.Timestamp, e.Result, ticket, rule, strings.TrimSpace(e.Reason))
		}
		return nil
	},
}

type guardianReportCounts struct {
	Allow int `json:"allow"`
	Warn  int `json:"warn"`
	Block int `json:"block"`
}

type guardianReportPayload struct {
	Total  int                                 `json:"total"`
	Counts guardianReportCounts                `json:"counts"`
	Recent []guardianevidence.DecisionEvidence `json:"recent"`
}

func loadGuardianEvidence(dir string) ([]guardianevidence.DecisionEvidence, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(ents))
	for _, ent := range ents {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		files = append(files, filepath.Join(dir, ent.Name()))
	}
	sort.Strings(files)
	out := make([]guardianevidence.DecisionEvidence, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var ev guardianevidence.DecisionEvidence
		if err := json.Unmarshal(b, &ev); err != nil {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

func buildGuardianReportPayload(entries []guardianevidence.DecisionEvidence) guardianReportPayload {
	p := guardianReportPayload{Total: len(entries), Recent: entries}
	for _, e := range entries {
		switch strings.ToUpper(strings.TrimSpace(e.Result)) {
		case "ALLOW":
			p.Counts.Allow++
		case "WARN":
			p.Counts.Warn++
		case "BLOCK":
			p.Counts.Block++
		}
	}
	return p
}

func init() {
	guardianReportCmd.Flags().BoolVar(&guardianReportJSON, "json", false, "print report as JSON")
	guardianMigrateCmd.Flags().BoolVar(&guardianMigrateApply, "apply", false, "write changes to config and scaffold missing flow file")
	guardianCmd.AddCommand(guardianReportCmd)
	guardianCmd.AddCommand(guardianMigrateCmd)
	rootCmd.AddCommand(guardianCmd)
}
