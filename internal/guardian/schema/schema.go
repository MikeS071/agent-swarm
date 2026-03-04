package schema

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Flow is the v2 guardian policy file model.
type Flow struct {
	Version     int          `yaml:"version"`
	Name        string       `yaml:"name"`
	Modes       Modes        `yaml:"modes"`
	State       State        `yaml:"state"`
	Phases      []Phase      `yaml:"phases"`
	Transitions []Transition `yaml:"transitions"`
}

type Modes struct {
	Default string `yaml:"default"`
}

type State struct {
	Initial  string   `yaml:"initial"`
	Terminal []string `yaml:"terminal"`
}

type Phase struct {
	ID     string   `yaml:"id"`
	States []string `yaml:"states"`
}

type Transition struct {
	From     string       `yaml:"from"`
	To       string       `yaml:"to"`
	Requires Requirements `yaml:"requires"`
}

type Requirements struct {
	Rules []RuleRef `yaml:"rules"`
}

type RuleRef struct {
	Type string `yaml:"type"`
}

type ValidationIssue struct {
	Field   string
	Message string
}

// ValidationError contains all schema issues found in a flow file.
type ValidationError struct {
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "flow validation failed"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Field, issue.Message))
	}
	return "flow validation failed: " + strings.Join(parts, "; ")
}

// Load parses and validates a flow.v2.yaml policy file.
func Load(path string) (*Flow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow file %s: %w", path, err)
	}
	var flow Flow
	if err := yaml.Unmarshal(b, &flow); err != nil {
		return nil, fmt.Errorf("parse flow file %s: %w", path, err)
	}

	if verr := Validate(&flow); verr != nil {
		return nil, verr
	}
	return &flow, nil
}

// Validate returns all schema violations for a flow policy.
func Validate(flow *Flow) *ValidationError {
	if flow == nil {
		return &ValidationError{Issues: []ValidationIssue{{Field: "flow", Message: "flow is nil"}}}
	}

	var issues []ValidationIssue
	add := func(field, msg string) {
		issues = append(issues, ValidationIssue{Field: field, Message: msg})
	}

	if flow.Version <= 0 {
		add("version", "must be a positive integer")
	}
	if strings.TrimSpace(flow.Name) == "" {
		add("name", "is required")
	}

	mode := strings.TrimSpace(flow.Modes.Default)
	if mode == "" {
		add("modes.default", "is required")
	} else if mode != "advisory" && mode != "enforce" {
		add("modes.default", `must be "advisory" or "enforce"`)
	}

	if strings.TrimSpace(flow.State.Initial) == "" {
		add("state.initial", "is required")
	}

	if len(flow.Transitions) == 0 {
		add("transitions", "must contain at least one transition")
	}

	knownStates := map[string]struct{}{}
	if s := strings.TrimSpace(flow.State.Initial); s != "" {
		knownStates[s] = struct{}{}
	}
	for _, terminal := range flow.State.Terminal {
		if s := strings.TrimSpace(terminal); s != "" {
			knownStates[s] = struct{}{}
		}
	}
	for i, phase := range flow.Phases {
		if strings.TrimSpace(phase.ID) == "" {
			add(fmt.Sprintf("phases[%d].id", i), "is required")
		}
		if len(phase.States) == 0 {
			add(fmt.Sprintf("phases[%d].states", i), "must contain at least one state")
		}
		for _, state := range phase.States {
			if s := strings.TrimSpace(state); s != "" {
				knownStates[s] = struct{}{}
			}
		}
	}

	allowedRules := supportedRules()
	for i, tr := range flow.Transitions {
		if from := strings.TrimSpace(tr.From); from == "" {
			add(fmt.Sprintf("transitions[%d].from", i), "is required")
		} else if _, ok := knownStates[from]; !ok {
			add(fmt.Sprintf("transition.from[%d]", i), fmt.Sprintf("unknown state %q", from))
		}

		if to := strings.TrimSpace(tr.To); to == "" {
			add(fmt.Sprintf("transitions[%d].to", i), "is required")
		} else if _, ok := knownStates[to]; !ok {
			add(fmt.Sprintf("transition.to[%d]", i), fmt.Sprintf("unknown state %q", to))
		}

		for rIdx, rule := range tr.Requires.Rules {
			ruleType := strings.TrimSpace(rule.Type)
			if ruleType == "" {
				add(fmt.Sprintf("transitions[%d].requires.rules[%d].type", i, rIdx), "is required")
				continue
			}
			if _, ok := allowedRules[ruleType]; !ok {
				add(
					fmt.Sprintf("unknown rule[%d,%d]", i, rIdx),
					fmt.Sprintf("unknown rule %q", ruleType),
				)
			}
		}
	}

	if len(issues) == 0 {
		return nil
	}
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Field < issues[j].Field
	})
	return &ValidationError{Issues: issues}
}

func supportedRules() map[string]struct{} {
	return map[string]struct{}{
		"prd_has_required_sections":          {},
		"prd_has_required_code_examples":     {},
		"spec_has_required_sections":         {},
		"spec_has_api_and_schema_examples":   {},
		"ticket_desc_has_scope_and_verify":   {},
		"phase_has_int_gap_tst_chain":        {},
		"all_build_tickets_done":             {},
		"no_open_critical_high":              {},
		"ticket_has_desc":                    {},
		"prompt_exists":                      {},
		"prompt_matches_template":            {},
		"profile_exists_if_set":              {},
		"branch_matches_regex":               {},
		"has_commit_ahead_of_base":           {},
		"no_failed_required_checks":          {},
	}
}
