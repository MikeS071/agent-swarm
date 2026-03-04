package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	d := t.TempDir()
	p := filepath.Join(d, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestLoadValidPolicy(t *testing.T) {
	t.Parallel()
	p := writeTemp(t, "flow.v2.yaml", `
version: 2
mode: advisory
enforcement_points: [before_spawn, before_mark_done, transition, post_build_complete]
rules:
  - id: ticket_desc_has_scope_and_verify
    enabled: true
    description: x
    severity: block
    enforcement_points: [before_spawn]
    target:
      kind: ticket
      source: swarm/tracker.json
      fields: [objective, scope, verify]
    check:
      type: ticket_fields
      params:
        required_fields: [objective, scope, verify]
    pass_when:
      op: all
      conditions:
        - metric: required_fields_present
          equals: true
    fail_reason: missing fields
    evidence:
      kind: json
      path: evidence/rule.json
overrides:
  enabled: true
  require_reason: true
  require_expiry: true
  max_duration_hours: 24
  store: approvals.json
events:
  file: guardian-events.jsonl
  include: [timestamp, result, reason]
`)

	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("version=%d want 2", got.Version)
	}
	if got.Mode != ModeAdvisory {
		t.Fatalf("mode=%q want advisory", got.Mode)
	}
}

func TestLoadInvalidPolicy(t *testing.T) {
	t.Parallel()
	p := writeTemp(t, "flow.v2.yaml", `
version: 1
mode: strict
enforcement_points: [unknown]
rules:
  - id: ""
    enabled: true
    severity: maybe
    enforcement_points: []
    target:
      kind: file
    check:
      type: ""
    pass_when:
      op: ""
      conditions: []
    fail_reason: ""
    evidence:
      path: ""
overrides:
  enabled: true
  require_expiry: true
  max_duration_hours: 0
events:
  file: ""
  include: []
`)

	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	mustContain := []string{"version", "mode", "enforcement_points", "rules[0].id", "events.file"}
	for _, s := range mustContain {
		if !strings.Contains(msg, s) {
			t.Fatalf("error %q missing %q", msg, s)
		}
	}
}
