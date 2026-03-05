package rules

import "testing"

func TestTicketDescHasScopeAndVerify(t *testing.T) {
	if !TicketDescHasScopeAndVerify("Implement guardian spawn checks. Scope: watchdog spawn path. Verify: go test ./...") {
		t.Fatal("expected ticket description to satisfy scope+verify rule")
	}
	missing := MissingTicketDescFields("Implement guardian spawn checks")
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing fields, got %v", missing)
	}
}

func TestPromptHasRequiredSections(t *testing.T) {
	prompt := `# ticket

## Objective
x

## Dependencies
y

## Scope
z

## Verify
go test ./...
`
	if !PromptHasRequiredSections(prompt) {
		t.Fatal("expected prompt to satisfy required sections")
	}

	missing := MissingPromptSections("# ticket\n\n## Objective\nonly")
	if len(missing) == 0 {
		t.Fatal("expected missing prompt sections")
	}
}
