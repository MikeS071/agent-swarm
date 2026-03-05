package prompts

import (
	"fmt"
	"regexp"
	"strings"
)

type Issue struct {
	Ticket  string `json:"ticket,omitempty"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

var (
	requiredSections = []string{"objective", "dependencies", "scope", "verify"}

	todoPattern           = regexp.MustCompile(`(?i)\bTODO\b`)
	tbdPattern            = regexp.MustCompile(`(?i)\bTBD\b`)
	addDetailsHerePattern = regexp.MustCompile(`(?i)add details here`)
	ellipsisPattern       = regexp.MustCompile(`<\s*\.\.\.\s*>`)
	fileExtPattern        = regexp.MustCompile(`\.[a-zA-Z0-9]{1,8}\b`)
)

func ValidatePrompt(ticketID string, body []byte, strict bool) []Issue {
	if !strict {
		return nil
	}
	content := strings.ReplaceAll(string(body), "\r\n", "\n")
	issues := make([]Issue, 0)
	sections := parseSections(content)

	for _, section := range requiredSections {
		if _, ok := sections[section]; !ok {
			issues = append(issues, Issue{
				Ticket:  ticketID,
				Rule:    "prompt_has_required_sections",
				Message: fmt.Sprintf("missing required section: %s", section),
			})
		}
	}

	placeholderIssues := findPlaceholderIssues(ticketID, content)
	issues = append(issues, placeholderIssues...)

	verifyBody := strings.TrimSpace(sections["verify"])
	if verifyBody == "" {
		issues = append(issues, Issue{
			Ticket:  ticketID,
			Rule:    "prompt_has_verify_command",
			Message: "verify section must contain a command",
		})
	}

	scopeBody := strings.TrimSpace(sections["scope"])
	if !hasExplicitFileScope(scopeBody) {
		issues = append(issues, Issue{
			Ticket:  ticketID,
			Rule:    "prompt_has_explicit_file_scope",
			Message: "scope section must include at least one explicit file path or pattern",
		})
	}

	return issues
}

func parseSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")
	current := ""
	var builder strings.Builder

	flush := func() {
		if current == "" {
			return
		}
		sections[current] = strings.TrimSpace(builder.String())
		builder.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			current = normalizeSectionName(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			continue
		}
		if current == "" {
			continue
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	flush()
	return sections
}

func normalizeSectionName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, ":")
	return name
}

func findPlaceholderIssues(ticketID, content string) []Issue {
	issues := make([]Issue, 0, 4)
	if todoPattern.MatchString(content) {
		issues = append(issues, Issue{Ticket: ticketID, Rule: "prompt_no_unresolved_placeholders", Message: "found placeholder marker: TODO"})
	}
	if tbdPattern.MatchString(content) {
		issues = append(issues, Issue{Ticket: ticketID, Rule: "prompt_no_unresolved_placeholders", Message: "found placeholder marker: TBD"})
	}
	if ellipsisPattern.MatchString(content) {
		issues = append(issues, Issue{Ticket: ticketID, Rule: "prompt_no_unresolved_placeholders", Message: "found placeholder marker: <...>"})
	}
	if addDetailsHerePattern.MatchString(content) {
		issues = append(issues, Issue{Ticket: ticketID, Rule: "prompt_no_unresolved_placeholders", Message: "found placeholder marker: Add details here"})
	}
	return issues
}

func hasExplicitFileScope(scopeBody string) bool {
	if strings.TrimSpace(scopeBody) == "" {
		return false
	}
	lines := strings.Split(scopeBody, "\n")
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		candidate = strings.TrimLeft(candidate, "-*0123456789. ")
		candidate = strings.Trim(candidate, "`")
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") || strings.Contains(candidate, "\\") {
			return true
		}
		if strings.Contains(candidate, "*") {
			return true
		}
		if fileExtPattern.MatchString(candidate) {
			return true
		}
	}
	return false
}
