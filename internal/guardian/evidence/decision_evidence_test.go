package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDecisionEvidenceCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteDecisionEvidence(dir, DecisionEvidence{
		Event:    "before_spawn",
		TicketID: "g4-02",
		Result:   "BLOCK",
		RuleID:   "ticket_desc_has_scope_and_verify",
		Reason:   "missing scope/verify",
		Context:  map[string]any{"profile": "code-agent"},
	})
	if err != nil {
		t.Fatalf("write decision evidence: %v", err)
	}
	if path == "" {
		t.Fatal("expected evidence path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("evidence file not found: %v", err)
	}
	if filepath.Base(filepath.Dir(path)) != "evidence" {
		t.Fatalf("expected evidence dir, got %s", path)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	var got DecisionEvidence
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if got.RuleID != "ticket_desc_has_scope_and_verify" || got.Result != "BLOCK" {
		t.Fatalf("unexpected evidence payload: %+v", got)
	}
}

func TestWriteDecisionEvidenceNoBaseDir(t *testing.T) {
	path, err := WriteDecisionEvidence("", DecisionEvidence{Result: "WARN"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Fatalf("expected empty path, got %s", path)
	}
}
