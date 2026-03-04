package tracker

import (
	"errors"
	"path/filepath"
	"testing"
)

func validV2Ticket() Ticket {
	return Ticket{
		ID:      "tp-01",
		Status:  "todo",
		Phase:   1,
		Depends: []string{},
		Type:    "feature",
		RunID:   "run-2026-03-04T10-32-00Z",
		Role:    "backend",
		Desc:    "Guardian schema parser + validation",

		Objective:           "Implement flow.v2 schema loader and validator with structured errors.",
		ScopeIn:             []string{"internal/guardian/schema/types.go"},
		ScopeOut:            []string{"No changes to watchdog runtime behavior"},
		FilesToTouch:        []string{"internal/guardian/schema/*.go"},
		ReferenceFiles:      []string{"internal/config/config.go"},
		ImplementationSteps: []string{"Define policy structs", "Implement validator with multi-error output"},
		TestsToAddOrUpdate:  []string{"internal/guardian/schema/validate_test.go"},
		VerifyCmd:           "go test ./internal/guardian/schema/...",
		AcceptanceCriteria:  []string{"Valid policy loads successfully"},
		Constraints:         []string{"No tracker format changes in this ticket"},
		SessionName:         "session-x",
		SessionBackend:      "codex",
		SessionModel:        "gpt-5.3-codex",
	}
}

func hasFieldError(errs ValidationErrors, ticketID, field string) bool {
	for _, e := range errs {
		if e.TicketID == ticketID && e.Field == field {
			return true
		}
	}
	return false
}

func TestValidateStrictAcceptsValidV2Ticket(t *testing.T) {
	t.Parallel()
	tr := New("proj", map[string]Ticket{
		"tp-01": validV2Ticket(),
	})

	if err := tr.Validate(ValidationOptions{Strict: true}); err != nil {
		t.Fatalf("Validate(strict=true) error: %v", err)
	}
}

func TestValidateStrictReportsMissingRequiredFields(t *testing.T) {
	t.Parallel()
	tk := validV2Ticket()
	tk.ID = ""
	tk.Role = ""
	tk.Objective = " "
	tk.ScopeIn = nil
	tk.ImplementationSteps = []string{"Only one"}
	tk.VerifyCmd = " "

	tr := New("proj", map[string]Ticket{"tp-01": tk})
	err := tr.Validate(ValidationOptions{Strict: true})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	if !hasFieldError(errs, "tp-01", "role") {
		t.Fatalf("expected role error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "id") {
		t.Fatalf("expected id error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "objective") {
		t.Fatalf("expected objective error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "scope_in") {
		t.Fatalf("expected scope_in error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "implementation_steps") {
		t.Fatalf("expected implementation_steps error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "verify_cmd") {
		t.Fatalf("expected verify_cmd error, got %#v", errs)
	}
}

func TestValidateStrictRejectsInvalidArraysAndCommands(t *testing.T) {
	t.Parallel()
	tk := validV2Ticket()
	tk.ScopeOut = []string{""}
	tk.FilesToTouch = []string{"not-a-path-pattern"}
	tk.VerifyCmd = "go test ./...\nrm -rf /"

	tr := New("proj", map[string]Ticket{"tp-01": tk})
	err := tr.Validate(ValidationOptions{Strict: true})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	if !hasFieldError(errs, "tp-01", "scope_out[0]") {
		t.Fatalf("expected scope_out[0] error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "files_to_touch[0]") {
		t.Fatalf("expected files_to_touch[0] error, got %#v", errs)
	}
	if !hasFieldError(errs, "tp-01", "verify_cmd") {
		t.Fatalf("expected verify_cmd error, got %#v", errs)
	}
}

func TestValidateStrictRejectsMismatchedTicketID(t *testing.T) {
	t.Parallel()
	tk := validV2Ticket()
	tk.ID = "tp-02"

	tr := New("proj", map[string]Ticket{"tp-01": tk})
	err := tr.Validate(ValidationOptions{Strict: true})
	if err == nil {
		t.Fatal("expected validation error")
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	if !hasFieldError(errs, "tp-01", "id") {
		t.Fatalf("expected id mismatch error, got %#v", errs)
	}
}

func TestLoadWithOptionsStrictToggleSupportsLegacyTickets(t *testing.T) {
	t.Parallel()
	path := writeTrackerFile(t, `{"project":"x","tickets":{"t1":{"status":"todo","phase":1,"depends":[]}}}`)

	if _, err := LoadWithOptions(path, ValidationOptions{Strict: false}); err != nil {
		t.Fatalf("LoadWithOptions(strict=false): %v", err)
	}
	if _, err := LoadWithOptions(path, ValidationOptions{Strict: true}); err == nil {
		t.Fatal("expected strict load to fail for legacy ticket")
	}
}

func TestSaveToWithOptionsStrictToggle(t *testing.T) {
	t.Parallel()
	base := filepath.Join(t.TempDir(), "tracker.json")

	legacy := New("x", map[string]Ticket{
		"t1": {Status: "todo", Phase: 1, Depends: []string{}},
	})
	if err := legacy.SaveToWithOptions(base, ValidationOptions{Strict: false}); err != nil {
		t.Fatalf("SaveToWithOptions(strict=false): %v", err)
	}
	if err := legacy.SaveToWithOptions(base+".strict", ValidationOptions{Strict: true}); err == nil {
		t.Fatal("expected strict save to fail for legacy ticket")
	}
}
