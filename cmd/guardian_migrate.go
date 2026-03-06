package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

const (
	guardianDefaultFlowFile = "swarm/flow.v2.yaml"
	guardianDefaultMode     = "advisory"
)

var guardianMigrateApply bool

type guardianMigrateReport struct {
	DryRun  bool
	Applied bool
	Changes []string
}

var guardianMigrateCmd = &cobra.Command{
	Use:          "migrate",
	Short:        "Migrate project to safe guardian defaults (advisory-first)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := runGuardianMigrate(cfgFile, guardianMigrateApply)
		if err != nil {
			return err
		}
		mode := "dry-run"
		if report.Applied {
			mode = "applied"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "guardian migrate (%s): %d change(s)\n", mode, len(report.Changes))
		for _, c := range report.Changes {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", c)
		}
		return nil
	},
}

func runGuardianMigrate(cfgPath string, apply bool) (guardianMigrateReport, error) {
	report := guardianMigrateReport{DryRun: !apply}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return report, err
	}
	repoRoot := cfg.Project.Repo
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = filepath.Dir(cfgPath)
	}
	flowAbs := filepath.Join(repoRoot, guardianDefaultFlowFile)

	raw, err := loadRawConfig(cfgPath)
	if err != nil {
		return report, err
	}
	guardian := getOrCreateSubtable(raw, "guardian")

	curEnabled := asBool(guardian["enabled"])
	curMode := strings.ToLower(strings.TrimSpace(asStringValue(guardian["mode"])))
	curFlow := strings.TrimSpace(asStringValue(guardian["flow_file"]))

	if curEnabled != true {
		report.Changes = append(report.Changes, fmt.Sprintf("set guardian.enabled=true (was %v)", curEnabled))
	}
	if curMode != guardianDefaultMode {
		if curMode == "" {
			report.Changes = append(report.Changes, "set guardian.mode=\"advisory\" (was unset)")
		} else {
			report.Changes = append(report.Changes, fmt.Sprintf("set guardian.mode=\"advisory\" (was %q)", curMode))
		}
	}
	if curFlow != guardianDefaultFlowFile {
		if curFlow == "" {
			report.Changes = append(report.Changes, "set guardian.flow_file=\"swarm/flow.v2.yaml\" (was unset)")
		} else {
			report.Changes = append(report.Changes, fmt.Sprintf("set guardian.flow_file=\"swarm/flow.v2.yaml\" (was %q)", curFlow))
		}
	}

	if _, err := os.Stat(flowAbs); err != nil {
		if os.IsNotExist(err) {
			report.Changes = append(report.Changes, "create swarm/flow.v2.yaml from default template")
		} else {
			return report, err
		}
	}

	if !apply {
		return report, nil
	}

	guardian["enabled"] = true
	guardian["mode"] = guardianDefaultMode
	guardian["flow_file"] = guardianDefaultFlowFile
	if err := writeRawConfig(cfgPath, raw); err != nil {
		return report, err
	}
	if err := ensureGuardianFlowFile(flowAbs); err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

func loadRawConfig(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func writeRawConfig(path string, raw map[string]any) error {
	b, err := toml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func getOrCreateSubtable(raw map[string]any, key string) map[string]any {
	if existing, ok := raw[key].(map[string]any); ok {
		return existing
	}
	table := map[string]any{}
	raw[key] = table
	return table
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asStringValue(v any) string {
	s, _ := v.(string)
	return s
}

func ensureGuardianFlowFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := assets.ReadFile("assets/flow.v2.yaml")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
