package rules

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	RuleTicketHasRequiredFields    = "ticket_has_required_fields"
	RulePromptHasRequiredSections  = "prompt_has_required_sections"
	RulePromptHasVerifyCommand     = "prompt_has_verify_command"
	RulePromptHasExplicitFileScope = "prompt_has_explicit_file_scope"
)

type Input struct {
	TicketID            string
	Profile             string
	PromptPath          string
	PromptBody          string
	VerifyCmd           string
	RequireExplicitRole bool
	RequireVerifyCmd    bool
}

type Violation struct {
	RuleID   string
	Reason   string
	Evidence map[string]any
}

func EvaluateBeforeSpawn(in Input) []Violation {
	out := make([]Violation, 0, 4)
	checks := []func(Input) *Violation{
		TicketHasRequiredFields,
		PromptHasRequiredSections,
		PromptHasVerifyCommand,
		PromptHasExplicitFileScope,
	}
	for _, check := range checks {
		if v := check(in); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

func TicketHasRequiredFields(in Input) *Violation {
	if !in.RequireExplicitRole {
		return nil
	}
	if strings.TrimSpace(in.Profile) != "" {
		return nil
	}
	return &Violation{
		RuleID: RuleTicketHasRequiredFields,
		Reason: "missing explicit role/profile",
		Evidence: map[string]any{
			"ticket_id": in.TicketID,
			"profile":   strings.TrimSpace(in.Profile),
		},
	}
}

func PromptHasRequiredSections(in Input) *Violation {
	prompt := strings.TrimSpace(in.PromptBody)
	if prompt == "" {
		return &Violation{
			RuleID: RulePromptHasRequiredSections,
			Reason: "prompt is empty",
			Evidence: map[string]any{
				"prompt_path": in.PromptPath,
			},
		}
	}
	required := []string{"Objective", "Verify", "Done Definition"}
	missing := make([]string, 0, len(required))
	for _, section := range required {
		if !hasSection(prompt, section) {
			missing = append(missing, strings.ToLower(section))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &Violation{
		RuleID: RulePromptHasRequiredSections,
		Reason: fmt.Sprintf("missing required prompt sections: %s", strings.Join(missing, ", ")),
		Evidence: map[string]any{
			"missing_sections": missing,
			"prompt_path":      in.PromptPath,
		},
	}
}

func PromptHasVerifyCommand(in Input) *Violation {
	if !in.RequireVerifyCmd {
		return nil
	}
	if strings.TrimSpace(in.VerifyCmd) != "" {
		return nil
	}
	prompt := strings.TrimSpace(in.PromptBody)
	verify, ok := sectionBody(prompt, "Verify")
	if !ok {
		return &Violation{
			RuleID: RulePromptHasVerifyCommand,
			Reason: "missing verify command",
			Evidence: map[string]any{
				"prompt_path": in.PromptPath,
				"section":     "verify",
			},
		}
	}
	if hasCommand(verify) {
		return nil
	}
	return &Violation{
		RuleID: RulePromptHasVerifyCommand,
		Reason: "verify section missing executable command",
		Evidence: map[string]any{
			"prompt_path": in.PromptPath,
			"section":     "verify",
		},
	}
}

func PromptHasExplicitFileScope(in Input) *Violation {
	prompt := strings.TrimSpace(in.PromptBody)
	if prompt == "" {
		return &Violation{
			RuleID: RulePromptHasExplicitFileScope,
			Reason: "missing explicit file scope",
			Evidence: map[string]any{
				"prompt_path": in.PromptPath,
			},
		}
	}
	titles := []string{"Files to touch", "File Scope", "Scope", "Your Scope"}
	hadScopeSection := false
	for _, title := range titles {
		body, ok := sectionBody(prompt, title)
		if !ok {
			continue
		}
		hadScopeSection = true
		if hasPathToken(body) {
			return nil
		}
	}
	if !hadScopeSection {
		return &Violation{
			RuleID: RulePromptHasExplicitFileScope,
			Reason: "missing explicit file scope section",
			Evidence: map[string]any{
				"prompt_path": in.PromptPath,
			},
		}
	}
	return &Violation{
		RuleID: RulePromptHasExplicitFileScope,
		Reason: "file scope section does not list explicit file paths",
		Evidence: map[string]any{
			"prompt_path": in.PromptPath,
		},
	}
}

func hasSection(prompt, title string) bool {
	_, ok := sectionBody(prompt, title)
	return ok
}

func sectionBody(prompt, title string) (string, bool) {
	if strings.TrimSpace(prompt) == "" {
		return "", false
	}
	re := regexp.MustCompile(`(?im)^##\s*` + regexp.QuoteMeta(title) + `\s*$`)
	loc := re.FindStringIndex(prompt)
	if loc == nil {
		return "", false
	}
	tail := prompt[loc[1]:]
	next := regexp.MustCompile(`(?im)^##\s+`).FindStringIndex(tail)
	if next == nil {
		return strings.TrimSpace(tail), true
	}
	return strings.TrimSpace(tail[:next[0]]), true
}

func hasCommand(section string) bool {
	if strings.TrimSpace(section) == "" {
		return false
	}
	lines := strings.Split(section, "\n")
	insideFence := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "```") {
			insideFence = !insideFence
			continue
		}
		if insideFence {
			return true
		}
		if strings.HasPrefix(line, "`") && strings.HasSuffix(line, "`") {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "`"), "`"))
			if commandLike(inner) {
				return true
			}
			continue
		}
		if commandLike(line) {
			return true
		}
	}
	return false
}

func commandLike(line string) bool {
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	if line == "" {
		return false
	}
	prefixes := []string{"go ", "npm ", "pnpm ", "yarn ", "make ", "pytest", "bash ", "./", "cargo ", "python ", "node "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return strings.Contains(line, " ./") || strings.HasPrefix(line, "./")
}

func hasPathToken(body string) bool {
	re := regexp.MustCompile(`(?m)(` + "`" + `)?[A-Za-z0-9._\-*]+(?:/[A-Za-z0-9._\-*]+)+(?:\.[A-Za-z0-9._-]+)?(` + "`" + `)?`)
	return re.FindStringIndex(body) != nil
}

func containsCI(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
