package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var decapodPattern = regexp.MustCompile(`(?i)\bdecapod\b`)

func TestEmbeddedAgentsTemplateHasNoDecapodReferences(t *testing.T) {
	data, err := assets.ReadFile("assets/AGENTS.md")
	if err != nil {
		t.Fatalf("read embedded AGENTS.md: %v", err)
	}
	if decapodPattern.Match(data) {
		t.Fatalf("embedded AGENTS.md contains decapod reference")
	}
}

func TestPromptFooterHasNoDecapodReferences(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "swarm", "prompt-footer.md"))
	if err != nil {
		t.Fatalf("read prompt footer: %v", err)
	}
	if decapodPattern.Match(data) {
		t.Fatalf("prompt footer contains decapod reference")
	}
}

func TestProjectDocsHaveNoDecapodReferences(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"user-guide.md",
		"lessons-learned.md",
		"AGENT-SWARM-V2-SPEC.md",
	}

	for _, name := range testCases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("..", "docs", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if decapodPattern.Match(data) {
				t.Fatalf("%s contains decapod reference", name)
			}
		})
	}
}
