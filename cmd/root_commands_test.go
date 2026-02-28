package cmd

import (
	"bytes"
	"testing"
)

func TestPromptsCommandIsRegistered(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"prompts", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("prompts command should be available, got error: %v\noutput:\n%s", err, out.String())
	}
}

func TestCleanupCommandIsRegistered(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"cleanup", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("cleanup command should be available, got error: %v\noutput:\n%s", err, out.String())
	}
}
