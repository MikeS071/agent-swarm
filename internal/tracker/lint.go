package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

var lintAllowedStatuses = map[string]struct{}{
	StatusTodo:    {},
	StatusRunning: {},
	StatusDone:    {},
	StatusFailed:  {},
	"blocked":    {},
}

// TicketValidationIssue captures one schema violation with a field path.
type TicketValidationIssue struct {
	TicketID string `json:"ticket_id,omitempty"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// TicketLintReport is the machine-friendly result returned by tickets lint.
type TicketLintReport struct {
	Strict      bool                  `json:"strict"`
	TicketCount int                   `json:"ticket_count"`
	IssueCount  int                   `json:"issue_count"`
	Issues      []TicketValidationIssue `json:"issues"`
}

// String returns a stable, human-readable representation of all issues.
func (r TicketLintReport) String() string {
	if len(r.Issues) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Issues))
	for _, issue := range r.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	return strings.Join(parts, "; ")
}

// LintTickets validates tracker ticket schema fields and returns structured issues.
func LintTickets(path string, strict bool) (TicketLintReport, error) {
	report := TicketLintReport{Strict: strict, Issues: make([]TicketValidationIssue, 0)}

	data, err := os.ReadFile(path)
	if err != nil {
		return report, fmt.Errorf("read tracker %s: %w", path, err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return report, fmt.Errorf("parse tracker %s: %w", path, err)
	}

	rawTickets, ok := root["tickets"]
	if !ok {
		appendLintIssue(&report, "", "tickets", "is required")
		report.IssueCount = len(report.Issues)
		return report, nil
	}

	tickets := map[string]json.RawMessage{}
	if err := json.Unmarshal(rawTickets, &tickets); err != nil {
		appendLintIssue(&report, "", "tickets", "must be an object")
		report.IssueCount = len(report.Issues)
		return report, nil
	}

	ids := make([]string, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	report.TicketCount = len(ids)

	for _, id := range ids {
		validateTicketRecord(&report, id, tickets[id], strict)
	}

	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Path == report.Issues[j].Path {
			if report.Issues[i].TicketID == report.Issues[j].TicketID {
				return report.Issues[i].Message < report.Issues[j].Message
			}
			return report.Issues[i].TicketID < report.Issues[j].TicketID
		}
		return report.Issues[i].Path < report.Issues[j].Path
	})
	report.IssueCount = len(report.Issues)
	return report, nil
}

func validateTicketRecord(report *TicketLintReport, id string, raw json.RawMessage, strict bool) {
	base := "tickets." + id
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		appendLintIssue(report, id, base, "must be an object")
		return
	}

	status := getStringField(report, fields, id, base, "status", true)
	if status != "" {
		if _, ok := lintAllowedStatuses[status]; !ok {
			appendLintIssue(report, id, base+".status", "must be one of todo|running|done|failed|blocked")
		}
	}

	phase := getIntField(report, fields, id, base, "phase", true)
	if phase <= 0 {
		appendLintIssue(report, id, base+".phase", "must be > 0")
	}

	_ = getStringSliceField(report, fields, id, base, "depends", true, 0, false)

	// v2 strict-required fields.
	_ = getStringField(report, fields, id, base, "type", strict)
	_ = getStringField(report, fields, id, base, "runId", strict)
	_ = getStringField(report, fields, id, base, "role", strict)
	_ = getStringField(report, fields, id, base, "objective", strict)
	_ = getStringSliceField(report, fields, id, base, "scope_in", strict, 1, false)
	_ = getStringSliceField(report, fields, id, base, "scope_out", strict, 1, false)
	_ = getStringSliceField(report, fields, id, base, "files_to_touch", strict, 1, true)
	_ = getStringSliceField(report, fields, id, base, "implementation_steps", strict, 2, false)
	_ = getStringSliceField(report, fields, id, base, "tests_to_add_or_update", strict, 1, false)
	_ = getStringField(report, fields, id, base, "verify_cmd", strict)
	_ = getStringSliceField(report, fields, id, base, "acceptance_criteria", strict, 1, false)
	_ = getStringSliceField(report, fields, id, base, "constraints", strict, 1, false)
}

func getStringField(report *TicketLintReport, fields map[string]json.RawMessage, ticketID, base, name string, required bool) string {
	raw, ok := fields[name]
	path := base + "." + name
	if !ok {
		if required {
			appendLintIssue(report, ticketID, path, "is required")
		}
		return ""
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		appendLintIssue(report, ticketID, path, "must be a string")
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		appendLintIssue(report, ticketID, path, "must not be empty")
		return ""
	}
	return value
}

func getIntField(report *TicketLintReport, fields map[string]json.RawMessage, ticketID, base, name string, required bool) int {
	raw, ok := fields[name]
	path := base + "." + name
	if !ok {
		if required {
			appendLintIssue(report, ticketID, path, "is required")
		}
		return 0
	}

	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		appendLintIssue(report, ticketID, path, "must be an integer")
		return 0
	}
	return value
}

func getStringSliceField(report *TicketLintReport, fields map[string]json.RawMessage, ticketID, base, name string, required bool, minLen int, pathLike bool) []string {
	raw, ok := fields[name]
	path := base + "." + name
	if !ok {
		if required {
			appendLintIssue(report, ticketID, path, "is required")
		}
		return nil
	}

	values := make([]string, 0)
	if err := json.Unmarshal(raw, &values); err != nil {
		appendLintIssue(report, ticketID, path, "must be an array of strings")
		return nil
	}
	if len(values) < minLen {
		appendLintIssue(report, ticketID, path, fmt.Sprintf("must contain at least %d item(s)", minLen))
	}
	for i, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			appendLintIssue(report, ticketID, fmt.Sprintf("%s[%d]", path, i), "must not be empty")
			continue
		}
		if pathLike && !looksLikePath(trimmed) {
			appendLintIssue(report, ticketID, fmt.Sprintf("%s[%d]", path, i), "must look like a file path or glob")
		}
	}
	return values
}

func looksLikePath(v string) bool {
	return strings.ContainsAny(v, "/.*")
}

func appendLintIssue(report *TicketLintReport, ticketID, path, message string) {
	report.Issues = append(report.Issues, TicketValidationIssue{
		TicketID: ticketID,
		Path:     path,
		Message:  message,
	})
}
