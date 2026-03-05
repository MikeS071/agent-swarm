package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfileMap_DefaultsWhenPathEmpty(t *testing.T) {
	m, err := LoadProfileMap("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["mem"] != "doc-updater" {
		t.Fatalf("expected default mem profile doc-updater, got %q", m["mem"])
	}
}

func TestLoadProfileMap_OverridesFromFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "lifecycle-policy.toml")
	if err := os.WriteFile(p, []byte(`[profiles.by_ticket_type]
mem = "code-reviewer"
review = "go-reviewer"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadProfileMap(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m["mem"]; got != "code-reviewer" {
		t.Fatalf("mem profile=%q want code-reviewer", got)
	}
	if got := m["review"]; got != "go-reviewer" {
		t.Fatalf("review profile=%q want go-reviewer", got)
	}
	if got := m["int"]; got != "code-agent" {
		t.Fatalf("int profile default fallback=%q want code-agent", got)
	}
}
