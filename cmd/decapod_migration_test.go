package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromptFooterTemplateHasNoDecapodReferences(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "swarm", "prompt-footer.md"))
	if err != nil {
		t.Fatalf("read prompt-footer template: %v", err)
	}
	content := strings.ToLower(string(data))

	for _, forbidden := range []string{
		"decapod",
		".decapod",
		"agent.init",
		"proof.validate",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("prompt-footer contains forbidden decapod reference %q", forbidden)
		}
	}
}

func TestMigrateRemoveDecapodScriptHappyPath(t *testing.T) {
	root := repoRoot(t)
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")

	if err := os.MkdirAll(filepath.Join(home, ".cargo", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir home cargo bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cargo", "bin", "decapod"), []byte("bin"), 0o755); err != nil {
		t.Fatalf("write decapod binary: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, ".decapod"), 0o755); err != nil {
		t.Fatalf("mkdir .decapod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".decapod", "README.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write .decapod marker: %v", err)
	}

	footer := `## Keep
This line should stay.
decapod validate && decapod session acquire && decapod rpc --op agent.init
After completing all work, run decapod rpc --op proof.validate
## End
`
	if err := os.MkdirAll(filepath.Join(repo, "swarm"), 0o755); err != nil {
		t.Fatalf("mkdir swarm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "swarm", "prompt-footer.md"), []byte(footer), 0o644); err != nil {
		t.Fatalf("write prompt footer: %v", err)
	}

	codex := "Before\nRun decapod validate first\nAfter\n"
	if err := os.WriteFile(filepath.Join(repo, "CODEX.md"), []byte(codex), 0o644); err != nil {
		t.Fatalf("write CODEX.md: %v", err)
	}

	out, err := runMigrationScript(root, home, repo)
	if err != nil {
		t.Fatalf("run migration script: %v\noutput:\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(home, ".cargo", "bin", "decapod")); !os.IsNotExist(err) {
		t.Fatalf("expected decapod binary removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".decapod")); !os.IsNotExist(err) {
		t.Fatalf("expected .decapod directory removed, stat err=%v", err)
	}

	updatedFooter, err := os.ReadFile(filepath.Join(repo, "swarm", "prompt-footer.md"))
	if err != nil {
		t.Fatalf("read updated prompt footer: %v", err)
	}
	updatedFooterText := strings.ToLower(string(updatedFooter))
	if strings.Contains(updatedFooterText, "decapod") {
		t.Fatalf("expected prompt footer decapod refs removed, got:\n%s", string(updatedFooter))
	}
	if !strings.Contains(string(updatedFooter), "This line should stay.") {
		t.Fatalf("prompt footer should preserve non-decapod lines")
	}

	updatedCodex, err := os.ReadFile(filepath.Join(repo, "CODEX.md"))
	if err != nil {
		t.Fatalf("read updated CODEX.md: %v", err)
	}
	if strings.Contains(strings.ToLower(string(updatedCodex)), "decapod") {
		t.Fatalf("expected CODEX decapod refs removed, got:\n%s", string(updatedCodex))
	}
	if !strings.Contains(string(updatedCodex), "Before") || !strings.Contains(string(updatedCodex), "After") {
		t.Fatalf("CODEX non-decapod content should be preserved")
	}
}

func TestMigrateRemoveDecapodScriptNoopWhenFilesMissing(t *testing.T) {
	root := repoRoot(t)
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")

	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	out, err := runMigrationScript(root, home, repo)
	if err != nil {
		t.Fatalf("expected noop run to succeed, err=%v\noutput:\n%s", err, out)
	}
}

func TestMigrateRemoveDecapodScriptErrorsForMissingRepo(t *testing.T) {
	root := repoRoot(t)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	missingRepo := filepath.Join(t.TempDir(), "does-not-exist")
	out, err := runMigrationScript(root, home, missingRepo)
	if err == nil {
		t.Fatalf("expected error for missing repo, output:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "does not exist") {
		t.Fatalf("expected missing repo error output, got:\n%s", out)
	}
}

func runMigrationScript(root, home, repo string) (string, error) {
	script := filepath.Join(root, "scripts", "migrate-remove-decapod.sh")
	cmd := exec.Command("bash", script, repo)
	cmd.Env = append(os.Environ(), "HOME="+home)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}
