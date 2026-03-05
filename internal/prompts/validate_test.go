package prompts

import "testing"

func TestValidatePromptStrict(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		strict    bool
		wantRules []string
		wantCount int
	}{
		{
			name: "strict valid prompt",
			body: `# sw-01
## Objective
Implement strict prompt validation.
## Dependencies
none
## Scope
- cmd/prompts_cmd.go
- internal/prompts/validate.go
## Verify
go test ./cmd/... -run PromptsValidate
`,
			strict:    true,
			wantCount: 0,
		},
		{
			name: "strict catches missing sections and checks",
			body: `# sw-01
## Objective
Implement strict prompt validation.
## Dependencies
none
`,
			strict: true,
			wantRules: []string{
				"prompt_has_required_sections",
				"prompt_has_verify_command",
				"prompt_has_explicit_file_scope",
			},
		},
		{
			name: "strict catches unresolved placeholders",
			body: `# sw-01
## Objective
TODO
## Dependencies
none
## Scope
- cmd/prompts_cmd.go
## Verify
go test ./cmd/... -run PromptsValidate
`,
			strict: true,
			wantRules: []string{
				"prompt_no_unresolved_placeholders",
			},
		},
		{
			name: "non-strict ignores placeholder and section quality checks",
			body: `# sw-01
## Objective
Add details here
`,
			strict:    false,
			wantCount: 0,
		},
		{
			name: "strict requires explicit file scope path",
			body: `# sw-01
## Objective
Implement strict prompt validation.
## Dependencies
none
## Scope
- update docs
## Verify
go test ./cmd/... -run PromptsValidate
`,
			strict: true,
			wantRules: []string{
				"prompt_has_explicit_file_scope",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := ValidatePrompt("sw-01", []byte(tt.body), tt.strict)
			if tt.wantCount == 0 && len(tt.wantRules) == 0 {
				if len(issues) != 0 {
					t.Fatalf("expected no issues, got: %+v", issues)
				}
				return
			}
			for _, rule := range tt.wantRules {
				if !hasRule(issues, rule) {
					t.Fatalf("expected rule %q in issues: %+v", rule, issues)
				}
			}
		})
	}
}

func hasRule(issues []Issue, rule string) bool {
	for _, issue := range issues {
		if issue.Rule == rule {
			return true
		}
	}
	return false
}
