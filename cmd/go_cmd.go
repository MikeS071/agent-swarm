package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	guardianengine "github.com/MikeS071/agent-swarm/internal/guardian/engine"
	guardianevidence "github.com/MikeS071/agent-swarm/internal/guardian/evidence"
	guardianschema "github.com/MikeS071/agent-swarm/internal/guardian/schema"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var goCmd = &cobra.Command{
	Use:   "go",
	Short: "Approve current phase gate and advance to next phase",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}

		d := dispatcher.New(cfg, tr)
		sig, _ := d.Evaluate()
		if sig != dispatcher.SignalPhaseGate {
			return fmt.Errorf("no phase gate reached (current signal: %s)", sig)
		}
		if err := runGuardianGoTransitionCheck(cfg, trackerPath, d.CurrentPhase()); err != nil {
			return err
		}

		sig, spawnable := d.ApprovePhaseGate()
		if err := tr.SaveTo(trackerPath); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "✅ Phase gate approved. Signal: %s\n", sig)
		if len(spawnable) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Spawnable tickets: %v\n", spawnable)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(goCmd)
}

func runGuardianGoTransitionCheck(cfg *config.Config, trackerPath string, phase int) error {
	if cfg == nil {
		return nil
	}
	flowPath := filepath.Join(strings.TrimSpace(cfg.Project.Repo), "swarm", "flow.v2.yaml")
	if strings.TrimSpace(cfg.Project.Repo) == "" {
		return nil
	}
	if _, err := os.Stat(flowPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat guardian flow file: %w", err)
	}

	mode := guardianModeFromFlowFile(flowPath)
	check := guardianengine.Check{
		Rule:   "flow_schema_valid",
		Passed: true,
		Reason: "flow schema valid",
		Target: fmt.Sprintf("phase:%d", phase),
	}
	if _, err := guardianschema.Load(flowPath); err != nil {
		check.Passed = false
		check.Reason = err.Error()
	}

	eventsPath := filepath.Join(filepath.Dir(trackerPath), "guardian-events.jsonl")
	writer := guardianevidence.NewEventWriter(eventsPath)
	decisions := guardianengine.Evaluate(mode, []guardianengine.Check{check})
	for _, decision := range decisions {
		if err := writer.AppendDecision(decision); err != nil {
			return err
		}
	}

	if guardianengine.Overall(decisions) == guardianengine.ResultBlock {
		reason := check.Reason
		if strings.TrimSpace(reason) == "" {
			reason = "policy check failed"
		}
		return fmt.Errorf("guardian blocked go transition: %s", reason)
	}
	return nil
}

func guardianModeFromFlowFile(path string) guardianengine.Mode {
	raw, err := os.ReadFile(path)
	if err != nil {
		return guardianengine.ModeAdvisory
	}

	var parsed struct {
		Modes struct {
			Default string `yaml:"default"`
		} `yaml:"modes"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err == nil {
		if strings.EqualFold(strings.TrimSpace(parsed.Modes.Default), string(guardianengine.ModeEnforce)) {
			return guardianengine.ModeEnforce
		}
		return guardianengine.ModeAdvisory
	}

	// Best-effort fallback for malformed YAML where mode can still be inferred.
	if strings.Contains(strings.ToLower(string(raw)), "default: enforce") {
		return guardianengine.ModeEnforce
	}
	return guardianengine.ModeAdvisory
}
