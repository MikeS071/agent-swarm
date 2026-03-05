package evidence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalStoreSetAndLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "approvals.json")
	s := NewApprovalStore(p)

	at := time.Date(2026, 3, 5, 11, 0, 0, 0, time.UTC)
	if err := s.Set("prd_approved", "human", "approved", at); err != nil {
		t.Fatalf("set approval: %v", err)
	}
	m, err := s.Load()
	if err != nil {
		t.Fatalf("load approvals: %v", err)
	}
	got, ok := m["prd_approved"]
	if !ok {
		t.Fatalf("missing prd_approved entry")
	}
	if got.By != "human" || got.At != at.Format(time.RFC3339) {
		t.Fatalf("unexpected entry: %+v", got)
	}
}

func TestApprovalStoreRejectsMissingFields(t *testing.T) {
	s := NewApprovalStore(filepath.Join(t.TempDir(), "approvals.json"))
	if err := s.Set("", "human", "", time.Time{}); err == nil {
		t.Fatal("expected error for missing key")
	}
	if err := s.Set("prd", "", "", time.Time{}); err == nil {
		t.Fatal("expected error for missing by")
	}
}

func TestApprovalStoreDelete(t *testing.T) {
	p := filepath.Join(t.TempDir(), "approvals.json")
	s := NewApprovalStore(p)
	if err := s.Set("prd_approved", "human", "approved", time.Now().UTC()); err != nil {
		t.Fatalf("set approval: %v", err)
	}
	if err := s.Delete("prd_approved"); err != nil {
		t.Fatalf("delete approval: %v", err)
	}
	m, err := s.Load()
	if err != nil {
		t.Fatalf("load approvals: %v", err)
	}
	if _, ok := m["prd_approved"]; ok {
		t.Fatal("expected prd_approved to be deleted")
	}
}

func TestApprovalStoreLoadInvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "approvals.json")
	if err := os.WriteFile(p, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	s := NewApprovalStore(p)
	if _, err := s.Load(); err == nil {
		t.Fatal("expected parse error")
	}
}
