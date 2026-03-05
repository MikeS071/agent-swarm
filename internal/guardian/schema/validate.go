package schema

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e))
	for _, issue := range e {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "; ")
}

func (e *ValidationErrors) add(path, message string) {
	*e = append(*e, ValidationError{Path: path, Message: message})
}

func (e ValidationErrors) sorted() ValidationErrors {
	if len(e) == 0 {
		return e
	}
	clone := append(ValidationErrors(nil), e...)
	sort.Slice(clone, func(i, j int) bool {
		if clone[i].Path == clone[j].Path {
			return clone[i].Message < clone[j].Message
		}
		return clone[i].Path < clone[j].Path
	})
	return clone
}

func Validate(p *FlowPolicy) error {
	var errs ValidationErrors
	if p == nil {
		return ValidationErrors{{Path: "policy", Message: "is nil"}}
	}

	if p.Version != 2 {
		errs.add("version", "must be 2")
	}
	if p.Mode != ModeAdvisory && p.Mode != ModeEnforce {
		errs.add("mode", "must be advisory or enforce")
	}
	if len(p.EnforcementPoints) == 0 {
		errs.add("enforcement_points", "must not be empty")
	}

	allowedEP := map[string]struct{}{
		"before_spawn":        {},
		"before_mark_done":    {},
		"transition":          {},
		"post_build_complete": {},
	}
	for i, ep := range p.EnforcementPoints {
		if _, ok := allowedEP[ep]; !ok {
			errs.add(fmt.Sprintf("enforcement_points[%d]", i), "unknown enforcement point")
		}
	}

	if len(p.Rules) == 0 {
		errs.add("rules", "must contain at least one rule")
	}

	seenRules := map[string]struct{}{}
	for i, r := range p.Rules {
		base := fmt.Sprintf("rules[%d]", i)
		if strings.TrimSpace(r.ID) == "" {
			errs.add(base+".id", "is required")
		} else {
			if _, ok := seenRules[r.ID]; ok {
				errs.add(base+".id", "must be unique")
			}
			seenRules[r.ID] = struct{}{}
		}

		if !isSeverity(r.Severity) {
			errs.add(base+".severity", "must be warn or block")
		}

		if len(r.EnforcementPoints) == 0 {
			errs.add(base+".enforcement_points", "must not be empty")
		}
		for j, ep := range r.EnforcementPoints {
			if _, ok := allowedEP[ep]; !ok {
				errs.add(fmt.Sprintf("%s.enforcement_points[%d]", base, j), "unknown enforcement point")
			}
		}

		if strings.TrimSpace(r.Target.Kind) == "" {
			errs.add(base+".target.kind", "is required")
		}
		switch r.Target.Kind {
		case "file":
			if len(r.Target.Paths) == 0 {
				errs.add(base+".target.paths", "is required for file targets")
			}
			if r.Target.Match != "" && r.Target.Match != "any" && r.Target.Match != "all" {
				errs.add(base+".target.match", "must be any or all")
			}
		case "ticket", "phase":
			if strings.TrimSpace(r.Target.Source) == "" {
				errs.add(base+".target.source", "is required for ticket/phase targets")
			}
		default:
			errs.add(base+".target.kind", "must be file, ticket, or phase")
		}

		if strings.TrimSpace(r.Check.Type) == "" {
			errs.add(base+".check.type", "is required")
		}
		if strings.TrimSpace(r.PassWhen.Op) == "" {
			errs.add(base+".pass_when.op", "is required")
		}
		if len(r.PassWhen.Conditions) == 0 {
			errs.add(base+".pass_when.conditions", "must not be empty")
		}
		for j, c := range r.PassWhen.Conditions {
			if strings.TrimSpace(c.Metric) == "" {
				errs.add(fmt.Sprintf("%s.pass_when.conditions[%d].metric", base, j), "is required")
			}
			if c.Equals == nil && c.GTE == nil && c.LTE == nil {
				errs.add(fmt.Sprintf("%s.pass_when.conditions[%d]", base, j), "requires one comparator: equals/gte/lte")
			}
		}

		if strings.TrimSpace(r.FailReason) == "" {
			errs.add(base+".fail_reason", "is required")
		}
		if strings.TrimSpace(r.Evidence.Path) == "" {
			errs.add(base+".evidence.path", "is required")
		}
	}

	if p.Overrides.Enabled {
		if p.Overrides.RequireExpiry && p.Overrides.MaxDurationHours <= 0 {
			errs.add("overrides.max_duration_hours", "must be > 0 when require_expiry is true")
		}
		if strings.TrimSpace(p.Overrides.Store) == "" {
			errs.add("overrides.store", "is required when overrides.enabled is true")
		}
	}

	if strings.TrimSpace(p.Events.File) == "" {
		errs.add("events.file", "is required")
	}
	if len(p.Events.Include) == 0 {
		errs.add("events.include", "must not be empty")
	}

	if len(errs) > 0 {
		return errs.sorted()
	}
	return nil
}

func isSeverity(v string) bool {
	return v == "warn" || v == "block"
}
