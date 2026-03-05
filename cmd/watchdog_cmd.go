package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type registryProjectRunResult struct {
	Project string `json:"project"`
	Config  string `json:"config"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

func loadProjectRegistry() (map[string]projectRegistryEntry, error) {
	path, err := projectsRegistryPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]projectRegistryEntry{}, nil
		}
		return nil, err
	}
	out := map[string]projectRegistryEntry{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveProjectRegistry(reg map[string]projectRegistryEntry) error {
	path, err := projectsRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

var watchdogRunAllDryRun bool
var watchdogRunAllJSON bool
var watchdogRunAllStrict bool
var watchdogRunAllPruneMissing bool

var watchdogCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "Watchdog utilities",
}

var watchdogRunAllOnceCmd = &cobra.Command{
	Use:          "run-all-once",
	Short:        "Run one watchdog pass for every registered project",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := loadProjectRegistry()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(reg))
		for name := range reg {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) == 0 {
			if watchdogRunAllJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"projects": 0, "failures": 0, "dry_run": watchdogRunAllDryRun, "results": []registryProjectRunResult{}})
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no registered projects")
			}
			return nil
		}

		results := make([]registryProjectRunResult, 0, len(names))
		failures := 0
		for _, name := range names {
			entry := reg[name]
			cfgPath := filepath.Join(entry.Repo, "swarm.toml")
			cfgPath = strings.TrimSpace(cfgPath)
			res := registryProjectRunResult{Project: name, Config: cfgPath}
			if _, err := os.Stat(cfgPath); err != nil {
				res.OK = false
				res.Error = fmt.Sprintf("missing config: %v", err)
				if watchdogRunAllPruneMissing {
					delete(reg, name)
					res.Error += " (pruned from registry)"
				}
				if watchdogRunAllStrict {
					failures++
				}
				results = append(results, res)
				continue
			}
			if err := runWatchWithConfigPath(cmd.Context(), cfgPath, "", true, watchdogRunAllDryRun); err != nil {
				res.OK = false
				res.Error = err.Error()
				if watchdogRunAllStrict {
					failures++
				}
			} else {
				res.OK = true
			}
			results = append(results, res)
		}

		if watchdogRunAllPruneMissing {
			if err := saveProjectRegistry(reg); err != nil && watchdogRunAllStrict {
				return fmt.Errorf("save pruned registry: %w", err)
			}
		}

		if watchdogRunAllJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"projects": len(results),
				"failures": failures,
				"dry_run":  watchdogRunAllDryRun,
				"results":  results,
			})
		} else {
			for _, r := range results {
				if r.OK {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OK   %s (%s)\n", r.Project, r.Config)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "FAIL %s (%s): %s\n", r.Project, r.Config, r.Error)
				}
			}
		}

		if failures > 0 {
			return fmt.Errorf("watchdog run-all-once: %d project(s) failed", failures)
		}
		return nil
	},
}

func init() {
	watchdogRunAllOnceCmd.Flags().BoolVar(&watchdogRunAllDryRun, "dry-run", false, "evaluate without mutating tracker state")
	watchdogRunAllOnceCmd.Flags().BoolVar(&watchdogRunAllJSON, "json", false, "output results as JSON")
	watchdogRunAllOnceCmd.Flags().BoolVar(&watchdogRunAllStrict, "strict", false, "return non-zero when any project fails")
	watchdogRunAllOnceCmd.Flags().BoolVar(&watchdogRunAllPruneMissing, "prune-missing", true, "remove registry entries whose configs no longer exist")
	watchdogCmd.AddCommand(watchdogRunAllOnceCmd)
	rootCmd.AddCommand(watchdogCmd)
}
