package prompts

import "testing"

func TestValidateStrictValidPrompt(t *testing.T) {
	content := `# TP-04

## Objective
Implement strict prompt validation.

## Files to touch
- cmd/prompts_cmd.go

## Implementation Steps
1. Add command.
2. Add tests.

## Verify
go test ./cmd/... ./internal/...

## Constraints
- Keep compatibility where reasonable.
`

	report := Validate(content, Options{Strict: true})
	if !report.Valid {
		t.Fatalf("expected valid report, failures: %#v", report.Failures)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("expected no failures, got %#v", report.Failures)
	}
}

func TestValidateStrictMissingVerifyAndFileScope(t *testing.T) {
	content := `# TP-04

## Objective
Implement strict prompt validation.

## Implementation Steps
1. Add command.
2. Add tests.

## Constraints
- Keep compatibility where reasonable.
`

	report := Validate(content, Options{Strict: true})
	if report.Valid {
		t.Fatal("expected invalid report")
	}
	assertHasRule(t, report, RuleMissingVerifyCommand)
	assertHasRule(t, report, RuleMissingFileScopeSection)
}

func TestValidateStrictDetectsPlaceholderMarkers(t *testing.T) {
	content := `# TP-04

## Objective
TODO

## Files to touch
- cmd/prompts_cmd.go

## Implementation Steps
1. Add command.
2. Add details here.

## Verify
go test ./cmd/... ./internal/...

## Constraints
- Replace <...>
- TBD
`

	report := Validate(content, Options{Strict: true})
	if report.Valid {
		t.Fatal("expected invalid report")
	}
	assertHasRule(t, report, RuleUnresolvedPlaceholder)
}

func assertHasRule(t *testing.T, report Report, want string) {
	t.Helper()
	for _, failure := range report.Failures {
		if failure.Rule == want {
			return
		}
	}
	t.Fatalf("expected rule %q in failures: %#v", want, report.Failures)
}
