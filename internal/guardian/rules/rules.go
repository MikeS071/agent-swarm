package rules

import "strings"

var requiredPromptSections = []string{
	"## objective",
	"## dependencies",
	"## scope",
	"## verify",
}

// MissingTicketDescFields returns required semantic fields missing from a ticket description.
func MissingTicketDescFields(desc string) []string {
	lower := strings.ToLower(strings.TrimSpace(desc))
	missing := make([]string, 0, 2)
	if !strings.Contains(lower, "scope") {
		missing = append(missing, "scope")
	}
	if !strings.Contains(lower, "verify") {
		missing = append(missing, "verify")
	}
	return missing
}

func TicketDescHasScopeAndVerify(desc string) bool {
	return len(MissingTicketDescFields(desc)) == 0
}

// MissingPromptSections returns required markdown section headers absent from prompt body.
func MissingPromptSections(prompt string) []string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	missing := make([]string, 0, len(requiredPromptSections))
	for _, sec := range requiredPromptSections {
		if !strings.Contains(lower, sec) {
			missing = append(missing, sec)
		}
	}
	return missing
}

func PromptHasRequiredSections(prompt string) bool {
	return len(MissingPromptSections(prompt)) == 0
}
