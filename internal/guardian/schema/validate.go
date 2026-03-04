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
	for _, v := range e {
		parts = append(parts, v.Error())
	}
	return strings.Join(parts, "; ")
}

func Validate(p *FlowPolicy) error {
	var errs ValidationErrors
	if p == nil {
		return ValidationErrors{{Path: "policy", Message: "is nil"}}
	}
	if p.Version != 2 {
		errs = append(errs, ValidationError{Path: "version", Message: "must be 2"})
	}
	if p.Mode != ModeAdvisory && p.Mode != ModeEnforce {
		errs = append(errs, ValidationError{Path: "mode", Message: "must be advisory or enforce"})
	}
	if len(p.EnforcementPoints) == 0 {
		errs = append(errs, ValidationError{Path: "enforcement_points", Message: "must not be empty"})
	}

	allowedEP := map[string]struct{}{
		"before_spawn":        {},
		"before_mark_done":    {},
		"transition":          {},
		"post_build_complete": {},
	}
	for i, ep := range p.EnforcementPoints {
		if _, ok := allowedEP[ep]; !ok {
			errs = append(errs, ValidationError{Path: fmt.Sprintf("enforcement_points[%d]", i), Message: "unknown enforcement point"})
		}
	}

	if len(p.Rules) == 0 {
		errs = append(errs, ValidationError{Path: "rules", Message: "must contain at least one rule"})
	}

	seenRules := map[string]struct{}{}
	for i, r := range p.Rules {
		base := fmt.Sprintf("rules[%d]", i)
		if strings.TrimSpace(r.ID) == "" {
			errs = append(errs, ValidationError{Path: base + ".id", Message: "is required"})
		} else {
			if _, ok := seenRules[r.ID]; ok {
				errs = append(errs, ValidationError{Path: base + ".id", Message: "must be unique"})
			}
			seenRules[r.ID] = struct{}{}
		}
		if !isSeverity(r.Severity) {
			errs = append(errs, ValidationError{Path: base + ".severity", Message: "must be warn or block"})
		}
		if len(r.EnforcementPoints) == 0 {
			errs = append(errs, ValidationError{Path: base + ".enforcement_points", Message: "must not be empty"})
		}
		for j, ep := range r.EnforcementPoints {
			if _, ok := allowedEP[ep]; !ok {
				errs = append(errs, ValidationError{Path: fmt.Sprintf("%s.enforcement_points[%d]", base, j), Message: "unknown enforcement point"})
			}
		}

		if strings.TrimSpace(r.Target.Kind) == "" {
			errs = append(errs, ValidationError{Path: base + ".target.kind", Message: "is required"})
		}
		switch r.Target.Kind {
		case "file":
			if len(r.Target.Paths) == 0 {
				errs = append(errs, ValidationError{Path: base + ".target.paths", Message: "is required for file targets"})
			}
			if r.Target.Match != "" && r.Target.Match != "any" && r.Target.Match != "all" {
				errs = append(errs, ValidationError{Path: base + ".target.match", Message: "must be any or all"})
			}
		case "ticket", "phase":
			if strings.TrimSpace(r.Target.Source) == "" {
				errs = append(errs, ValidationError{Path: base + ".target.source", Message: "is required for ticket/phase targets"})
			}
		default:
			errs = append(errs, ValidationError{Path: base + ".target.kind", Message: "must be file, ticket, or phase"})
		}

		if strings.TrimSpace(r.Check.Type) == "" {
			errs = append(errs, ValidationError{Path: base + ".check.type", Message: "is required"})
		}
		if strings.TrimSpace(r.PassWhen.Op) == "" {
			errs = append(errs, ValidationError{Path: base + ".pass_when.op", Message: "is required"})
		}
		if len(r.PassWhen.Conditions) == 0 {
			errs = append(errs, ValidationError{Path: base + ".pass_when.conditions", Message: "must not be empty"})
		}
		for j, c := range r.PassWhen.Conditions {
			if strings.TrimSpace(c.Metric) == "" {
				errs = append(errs, ValidationError{Path: fmt.Sprintf("%s.pass_when.conditions[%d].metric", base, j), Message: "is required"})
			}
			if c.Equals == nil && c.GTE == nil && c.LTE == nil {
				errs = append(errs, ValidationError{Path: fmt.Sprintf("%s.pass_when.conditions[%d]", base, j), Message: "requires one comparator: equals/gte/lte"})
			}
		}
		if strings.TrimSpace(r.FailReason) == "" {
			errs = append(errs, ValidationError{Path: base + ".fail_reason", Message: "is required"})
		}
		if strings.TrimSpace(r.Evidence.Path) == "" {
			errs = append(errs, ValidationError{Path: base + ".evidence.path", Message: "is required"})
		}
	}

	if p.Overrides.Enabled {
		if p.Overrides.RequireExpiry && p.Overrides.MaxDurationHours <= 0 {
			errs = append(errs, ValidationError{Path: "overrides.max_duration_hours", Message: "must be > 0 when require_expiry is true"})
		}
		if strings.TrimSpace(p.Overrides.Store) == "" {
			errs = append(errs, ValidationError{Path: "overrides.store", Message: "is required when overrides.enabled is true"})
		}
	}

	if strings.TrimSpace(p.Events.File) == "" {
		errs = append(errs, ValidationError{Path: "events.file", Message: "is required"})
	}
	if len(p.Events.Include) == 0 {
		errs = append(errs, ValidationError{Path: "events.include", Message: "must not be empty"})
	}

	if len(errs) > 0 {
		sort.Slice(errs, func(i, j int) bool { return errs[i].Path < errs[j].Path })
		return errs
	}
	return nil
}

func isSeverity(v string) bool {
	return v == "warn" || v == "block"
}
