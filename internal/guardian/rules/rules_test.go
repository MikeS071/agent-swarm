package rules

import "testing"

func TestTicketHasRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		in             Input
		want           bool
		rule           string
		reasonContains string
	}{
		{
			name: "passes when profile exists and required",
			in:   Input{Profile: "code-agent", RequireExplicitRole: true},
			want: false,
		},
		{
			name:           "fails when required profile missing",
			in:             Input{RequireExplicitRole: true},
			want:           true,
			rule:           RuleTicketHasRequiredFields,
			reasonContains: "missing explicit role/profile",
		},
		{
			name: "passes when profile missing but requirement disabled",
			in:   Input{RequireExplicitRole: false},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := TicketHasRequiredFields(tc.in)
			if tc.want && got == nil {
				t.Fatalf("expected violation")
			}
			if !tc.want && got != nil {
				t.Fatalf("expected no violation, got %#v", got)
			}
			if !tc.want {
				return
			}
			if got.RuleID != tc.rule {
				t.Fatalf("rule = %q, want %q", got.RuleID, tc.rule)
			}
			if tc.reasonContains != "" && !containsCI(got.Reason, tc.reasonContains) {
				t.Fatalf("reason = %q, want contain %q", got.Reason, tc.reasonContains)
			}
		})
	}
}

func TestPromptHasRequiredSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{
			name: "passes with objective verify and done definition sections",
			in: Input{PromptBody: `# TP-09

## Objective
ship it

## Verify
` + "```bash" + `
go test ./...
` + "```" + `

## Done Definition
all green
`},
			want: false,
		},
		{
			name: "fails when verify section missing",
			in: Input{PromptBody: `# TP-09

## Objective
ship it

## Done Definition
all green
`},
			want: true,
		},
		{
			name: "passes with case-insensitive section headings",
			in: Input{PromptBody: `# TP-09

## objective
ship it

## verify
` + "```bash" + `
go test ./...
` + "```" + `

## done definition
all green
`},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := PromptHasRequiredSections(tc.in)
			if tc.want && got == nil {
				t.Fatalf("expected violation")
			}
			if !tc.want && got != nil {
				t.Fatalf("expected no violation, got %#v", got)
			}
		})
	}
}

func TestPromptHasVerifyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{
			name: "passes when verify section contains command block",
			in: Input{RequireVerifyCmd: true, PromptBody: `## Verify
` + "```bash" + `
go test ./internal/guardian/...
` + "```" + `
`},
			want: false,
		},
		{
			name: "fails when verify section has no command and no fallback",
			in: Input{RequireVerifyCmd: true, PromptBody: `## Verify
follow project verify command
`},
			want: true,
		},
		{
			name: "passes when explicit ticket verify command exists",
			in: Input{RequireVerifyCmd: true, VerifyCmd: "go test ./...", PromptBody: `## Verify
follow project verify command
`},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := PromptHasVerifyCommand(tc.in)
			if tc.want && got == nil {
				t.Fatalf("expected violation")
			}
			if !tc.want && got != nil {
				t.Fatalf("expected no violation, got %#v", got)
			}
		})
	}
}

func TestPromptHasExplicitFileScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Input
		want bool
	}{
		{
			name: "passes when files-to-touch section lists concrete paths",
			in: Input{PromptBody: `## Files to touch
- internal/guardian/rules/rules.go
- internal/watchdog/watchdog.go
`},
			want: false,
		},
		{
			name: "fails when scope section has no explicit paths",
			in: Input{PromptBody: `## Scope
- improve architecture and quality
`},
			want: true,
		},
		{
			name: "passes with alternate heading and backticked paths",
			in:   Input{PromptBody: "## Your Scope\n- `internal/config/config.go`\n"},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := PromptHasExplicitFileScope(tc.in)
			if tc.want && got == nil {
				t.Fatalf("expected violation")
			}
			if !tc.want && got != nil {
				t.Fatalf("expected no violation, got %#v", got)
			}
		})
	}
}
