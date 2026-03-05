package prompts

import (
	"regexp"
	"strings"
)

const (
	RuleMissingRequiredSection  = "missing_required_section"
	RuleMissingVerifyCommand    = "missing_verify_command"
	RuleMissingFileScopeSection = "missing_file_scope_section"
	RuleUnresolvedPlaceholder   = "unresolved_placeholder"
)

type Options struct {
	Strict bool
}

type Failure struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type Report struct {
	Valid    bool      `json:"valid"`
	Failures []Failure `json:"failures,omitempty"`
}

var (
	todoPattern       = regexp.MustCompile(`(?i)\bTODO\b`)
	tbdPattern        = regexp.MustCompile(`(?i)\bTBD\b`)
	addDetailsPattern = regexp.MustCompile(`(?i)\bAdd details here\b`)
	ellipsisPattern   = regexp.MustCompile(`<\s*\.\.\.\s*>`)
)

func Validate(content string, opts Options) Report {
	sections := parseSections(content)
	failures := make([]Failure, 0)

	requiredSections := []string{"objective", "implementation steps", "constraints"}
	for _, section := range requiredSections {
		if strings.TrimSpace(sections[section]) == "" {
			failures = append(failures, Failure{
				Rule:    RuleMissingRequiredSection,
				Message: "missing section: " + section,
			})
		}
	}

	if !hasVerifySection(sections) {
		failures = append(failures, Failure{
			Rule:    RuleMissingVerifyCommand,
			Message: "missing verify command section",
		})
	}

	if !hasFileScopeSection(sections) {
		failures = append(failures, Failure{
			Rule:    RuleMissingFileScopeSection,
			Message: "missing file scope section",
		})
	}

	if opts.Strict {
		placeholderFailures := placeholderFailures(content)
		failures = append(failures, placeholderFailures...)
	}

	return Report{Valid: len(failures) == 0, Failures: failures}
}

func parseSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	current := ""
	var body strings.Builder
	flush := func() {
		if current == "" {
			return
		}
		sections[current] = strings.TrimSpace(body.String())
		body.Reset()
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			flush()
			current = normalizeHeading(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if current != "" {
			body.WriteString(raw)
			body.WriteByte('\n')
		}
	}
	flush()
	return sections
}

func normalizeHeading(heading string) string {
	trimmed := strings.TrimSpace(heading)
	trimmed = strings.TrimSuffix(trimmed, ":")
	fields := strings.Fields(strings.ToLower(trimmed))
	return strings.Join(fields, " ")
}

func hasVerifySection(sections map[string]string) bool {
	for heading, body := range sections {
		if strings.TrimSpace(body) == "" {
			continue
		}
		if heading == "verify" || heading == "verify command" {
			return true
		}
	}
	return false
}

func hasFileScopeSection(sections map[string]string) bool {
	for heading, body := range sections {
		if strings.TrimSpace(body) == "" {
			continue
		}
		if strings.Contains(heading, "files to touch") || heading == "scope in" || heading == "your scope" || heading == "file scope" {
			return true
		}
	}
	return false
}

func placeholderFailures(content string) []Failure {
	failures := make([]Failure, 0)
	if todoPattern.MatchString(content) {
		failures = append(failures, Failure{
			Rule:    RuleUnresolvedPlaceholder,
			Message: "found unresolved placeholder marker: TODO",
		})
	}
	if tbdPattern.MatchString(content) {
		failures = append(failures, Failure{
			Rule:    RuleUnresolvedPlaceholder,
			Message: "found unresolved placeholder marker: TBD",
		})
	}
	if ellipsisPattern.MatchString(content) {
		failures = append(failures, Failure{
			Rule:    RuleUnresolvedPlaceholder,
			Message: "found unresolved placeholder marker: <...>",
		})
	}
	if addDetailsPattern.MatchString(content) {
		failures = append(failures, Failure{
			Rule:    RuleUnresolvedPlaceholder,
			Message: "found unresolved placeholder marker: Add details here",
		})
	}
	return failures
}
