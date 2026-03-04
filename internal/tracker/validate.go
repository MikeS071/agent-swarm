package tracker

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationOptions struct {
	Strict bool
}

type ValidationError struct {
	TicketID string
	Field    string
	Message  string
}

func (e ValidationError) Error() string {
	if e.TicketID == "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("tickets[%s].%s: %s", e.TicketID, e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e))
	for _, v := range e {
		parts = append(parts, v.Error())
	}
	return strings.Join(parts, "; ")
}

func (t *Tracker) Validate(opts ValidationOptions) error {
	if t == nil {
		return ValidationErrors{{Field: "tracker", Message: "is nil"}}
	}
	if !opts.Strict {
		return nil
	}

	var errs ValidationErrors
	if strings.TrimSpace(t.Project) == "" {
		errs = append(errs, ValidationError{Field: "project", Message: "is required"})
	}
	for id, tk := range t.Tickets {
		validateStrictTicket(id, tk, &errs)
	}
	if len(errs) == 0 {
		return nil
	}
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].TicketID != errs[j].TicketID {
			return errs[i].TicketID < errs[j].TicketID
		}
		return errs[i].Field < errs[j].Field
	})
	return errs
}

func validateStrictTicket(id string, tk Ticket, errs *ValidationErrors) {
	if strings.TrimSpace(id) == "" {
		*errs = append(*errs, ValidationError{TicketID: id, Field: "id", Message: "is required"})
	}
	if tk.ID != "" && tk.ID != id {
		*errs = append(*errs, ValidationError{TicketID: id, Field: "id", Message: "must match ticket key"})
	}
	if strings.TrimSpace(tk.Status) == "" {
		*errs = append(*errs, ValidationError{TicketID: id, Field: "status", Message: "is required"})
	} else if _, ok := validStatuses[tk.Status]; !ok {
		*errs = append(*errs, ValidationError{TicketID: id, Field: "status", Message: "must be one of todo, running, done, failed, blocked"})
	}
	if tk.Phase <= 0 {
		*errs = append(*errs, ValidationError{TicketID: id, Field: "phase", Message: "must be > 0"})
	}
	requiredString(id, "type", tk.Type, errs)
	requiredString(id, "runId", tk.RunID, errs)
	requiredString(id, "role", tk.Role, errs)
	requiredString(id, "desc", tk.Desc, errs)
	requiredString(id, "objective", tk.Objective, errs)
	validateStringSlice(id, "scope_in", tk.ScopeIn, 1, false, errs)
	validateStringSlice(id, "scope_out", tk.ScopeOut, 1, false, errs)
	validateStringSlice(id, "files_to_touch", tk.FilesToTouch, 1, true, errs)
	validateStringSlice(id, "implementation_steps", tk.ImplementationSteps, 2, false, errs)
	validateStringSlice(id, "tests_to_add_or_update", tk.TestsToAddOrUpdate, 1, false, errs)
	requiredVerifyCommand(id, tk.VerifyCmd, errs)
	validateStringSlice(id, "acceptance_criteria", tk.AcceptanceCriteria, 1, false, errs)
	validateStringSlice(id, "constraints", tk.Constraints, 1, false, errs)
}

func requiredString(ticketID, field, value string, errs *ValidationErrors) {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, ValidationError{TicketID: ticketID, Field: field, Message: "is required"})
	}
}

func requiredVerifyCommand(ticketID, cmd string, errs *ValidationErrors) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		*errs = append(*errs, ValidationError{TicketID: ticketID, Field: "verify_cmd", Message: "is required"})
		return
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		*errs = append(*errs, ValidationError{TicketID: ticketID, Field: "verify_cmd", Message: "must be a single-line shell command"})
	}
}

func validateStringSlice(ticketID, field string, values []string, minLen int, pathLike bool, errs *ValidationErrors) {
	if len(values) < minLen {
		msg := "must not be empty"
		if minLen > 1 {
			msg = fmt.Sprintf("must contain at least %d entries", minLen)
		}
		*errs = append(*errs, ValidationError{TicketID: ticketID, Field: field, Message: msg})
		return
	}
	for i, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			*errs = append(*errs, ValidationError{TicketID: ticketID, Field: fmt.Sprintf("%s[%d]", field, i), Message: "must not be empty"})
			continue
		}
		if pathLike && !isPathLikePattern(trimmed) {
			*errs = append(*errs, ValidationError{TicketID: ticketID, Field: fmt.Sprintf("%s[%d]", field, i), Message: "must be a path-like pattern"})
		}
	}
}

func isPathLikePattern(v string) bool {
	if strings.ContainsAny(v, " \t\r\n") {
		return false
	}
	return strings.Contains(v, "/") || strings.Contains(v, ".") || strings.Contains(v, "*")
}
