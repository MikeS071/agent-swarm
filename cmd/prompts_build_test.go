package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type promptsBuildManifestFixture struct {
	TicketID   string `json:"ticket_id"`
	OutputPath string `json:"output_path"`
	OutputSHA  string `json:"output_sha256"`
	Layers     []struct {
		Name       string `json:"name"`
		SourcePath string `json:"source_path,omitempty"`
		SHA256     string `json:"sha256"`
	} `json:"layers"`
}

func TestPromptsBuildSingleTicketDeterministic(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePromptsBuildFixture(t, repo, map[string]string{
		"sw-01": "# sw-01\n\nImplement deterministic compiler\n",
	})

	if _, err := runSwarmCommand(t, cfgPath, "prompts", "build", "sw-01"); err != nil {
		t.Fatalf("first prompts build failed: %v", err)
	}
	promptPath := filepath.Join(repo, "swarm", "prompts", "sw-01.md")
	manifestPath := filepath.Join(repo, "swarm", "prompts", "sw-01.manifest.json")

	firstPrompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read first prompt: %v", err)
	}
	firstManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}

	if _, err := runSwarmCommand(t, cfgPath, "prompts", "build", "sw-01"); err != nil {
		t.Fatalf("second prompts build failed: %v", err)
	}

	secondPrompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read second prompt: %v", err)
	}
	secondManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read second manifest: %v", err)
	}

	if !bytes.Equal(firstPrompt, secondPrompt) {
		t.Fatalf("compiled prompt changed between builds\nfirst:\n%s\nsecond:\n%s", string(firstPrompt), string(secondPrompt))
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatalf("manifest changed between builds\nfirst:\n%s\nsecond:\n%s", string(firstManifest), string(secondManifest))
	}

	promptText := string(secondPrompt)
	assertContainsInOrder(t, promptText,
		"# Governance Rules",
		"# Project Specification",
		"# Code Agent Profile",
		"Implement deterministic compiler",
		"# Prompt Footer",
	)
}

func TestPromptsBuildManifestContent(t *testing.T) {
	repo := t.TempDir()
	rawTicket := "# sw-01\n\nImplement deterministic compiler\n"
	cfgPath := writePromptsBuildFixture(t, repo, map[string]string{
		"sw-01": rawTicket,
	})

	if _, err := runSwarmCommand(t, cfgPath, "prompts", "build", "sw-01"); err != nil {
		t.Fatalf("prompts build failed: %v", err)
	}

	manifestPath := filepath.Join(repo, "swarm", "prompts", "sw-01.manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest promptsBuildManifestFixture
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	if manifest.TicketID != "sw-01" {
		t.Fatalf("ticket_id = %q, want sw-01", manifest.TicketID)
	}
	if manifest.OutputPath != "swarm/prompts/sw-01.md" {
		t.Fatalf("output_path = %q, want swarm/prompts/sw-01.md", manifest.OutputPath)
	}
	if len(manifest.Layers) != 5 {
		t.Fatalf("layers len = %d, want 5", len(manifest.Layers))
	}

	wantNames := []string{"governance", "specification", "profile", "ticket", "footer"}
	gotNames := make([]string, 0, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		gotNames = append(gotNames, layer.Name)
		if strings.TrimSpace(layer.SHA256) == "" {
			t.Fatalf("layer %q has empty sha256", layer.Name)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("layer names = %v, want %v", gotNames, wantNames)
	}

	wantTicketHash := testSHA256Hex([]byte(rawTicket))
	if manifest.Layers[3].SHA256 != wantTicketHash {
		t.Fatalf("ticket layer sha256 = %q, want %q", manifest.Layers[3].SHA256, wantTicketHash)
	}

	compiledPrompt, err := os.ReadFile(filepath.Join(repo, manifest.OutputPath))
	if err != nil {
		t.Fatalf("read compiled prompt: %v", err)
	}
	if manifest.OutputSHA != testSHA256Hex(compiledPrompt) {
		t.Fatalf("output_sha256 = %q, want %q", manifest.OutputSHA, testSHA256Hex(compiledPrompt))
	}
}

func TestPromptsBuildAllModeBuildsSortedTicketSet(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePromptsBuildFixture(t, repo, map[string]string{
		"sw-02": "# sw-02\n\nSecond\n",
		"sw-01": "# sw-01\n\nFirst\n",
	})

	out, err := runSwarmCommand(t, cfgPath, "prompts", "build", "--all")
	if err != nil {
		t.Fatalf("prompts build --all failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"built sw-01", "built sw-02"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("output lines = %v, want %v", lines, want)
	}

	for _, id := range []string{"sw-01", "sw-02"} {
		manifestPath := filepath.Join(repo, "swarm", "prompts", id+".manifest.json")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("manifest missing for %s: %v", id, err)
		}
	}
}

func TestPromptsBuildArgumentValidation(t *testing.T) {
	repo := t.TempDir()
	cfgPath := writePromptsBuildFixture(t, repo, map[string]string{
		"sw-01": "# sw-01\n\nTest\n",
	})

	tests := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{
			name:       "no ticket and no all",
			args:       []string{"prompts", "build"},
			wantErrSub: "expected exactly one ticket id or --all",
		},
		{
			name:       "ticket and all",
			args:       []string{"prompts", "build", "sw-01", "--all"},
			wantErrSub: "expected exactly one ticket id or --all",
		},
		{
			name:       "unknown ticket",
			args:       []string{"prompts", "build", "sw-99"},
			wantErrSub: "ticket \"sw-99\" not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runSwarmCommand(t, cfgPath, tc.args...)
			if err == nil {
				t.Fatalf("expected error for args %v", tc.args)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

func writePromptsBuildFixture(t *testing.T, repo string, ticketPrompts map[string]string) string {
	t.Helper()

	writePath(t, filepath.Join(repo, "AGENTS.md"), "# Governance Rules\n")
	writePath(t, filepath.Join(repo, "SPEC.md"), "# Project Specification\n")
	writePath(t, filepath.Join(repo, ".agents", "profiles", "code-agent.md"), "# Code Agent Profile\n")
	writePath(t, filepath.Join(repo, "swarm", "prompt-footer.md"), "# Prompt Footer\n")

	for id, body := range ticketPrompts {
		writePath(t, filepath.Join(repo, "swarm", "prompts", id+".md"), body)
	}

	trackerTickets := make([]string, 0, len(ticketPrompts))
	for id := range ticketPrompts {
		trackerTickets = append(trackerTickets, "\""+id+"\": {\"status\": \"todo\", \"phase\": 1, \"depends\": [], \"branch\": \"feat/"+id+"\", \"desc\": \""+id+"\", \"profile\": \"code-agent\", \"verify_cmd\": \"go test ./...\"}")
	}
	trackerBody := "{\n  \"project\": \"test\",\n  \"tickets\": {\n    " + strings.Join(trackerTickets, ",\n    ") + "\n  }\n}\n"
	writePath(t, filepath.Join(repo, "swarm", "tracker.json"), trackerBody)

	cfgPath := filepath.Join(repo, "swarm.toml")
	cfg := "[project]\n" +
		"name = \"test\"\n" +
		"repo = \".\"\n" +
		"tracker = \"swarm/tracker.json\"\n" +
		"prompt_dir = \"swarm/prompts\"\n" +
		"spec_file = \"SPEC.md\"\n\n" +
		"[backend]\n" +
		"type = \"codex-tmux\"\n"
	writePath(t, cfgPath, cfg)
	return cfgPath
}

func runSwarmCommand(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	if err := promptsBuildCmd.Flags().Set("all", "false"); err != nil {
		t.Fatalf("reset prompts build --all flag: %v", err)
	}

	fullArgs := make([]string, 0, len(args)+2)
	fullArgs = append(fullArgs, "--config", cfgPath)
	fullArgs = append(fullArgs, args...)
	rootCmd.SetArgs(fullArgs)

	err := rootCmd.Execute()
	return strings.TrimSpace(out.String()), err
}

func assertContainsInOrder(t *testing.T, text string, snippets ...string) {
	t.Helper()
	prev := -1
	for _, snippet := range snippets {
		idx := strings.Index(text, snippet)
		if idx < 0 {
			t.Fatalf("missing snippet %q in output", snippet)
		}
		if idx <= prev {
			t.Fatalf("snippet %q out of order", snippet)
		}
		prev = idx
	}
}

func testSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
