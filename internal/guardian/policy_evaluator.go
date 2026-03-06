package guardian

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/guardian/rules"
	"github.com/MikeS071/agent-swarm/internal/guardian/schema"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type PolicyEvaluator struct {
	mode      string
	repoRoot  string
	tracker   string
	specFile  string
	policy    *schema.FlowPolicy
	policyErr error
}

func NewPolicyEvaluator(cfg *config.Config) Evaluator {
	if cfg == nil {
		return StrictEvaluator{}
	}
	e := &PolicyEvaluator{
		mode:     strings.ToLower(strings.TrimSpace(cfg.Guardian.Mode)),
		repoRoot: strings.TrimSpace(cfg.Project.Repo),
		tracker:  strings.TrimSpace(cfg.Project.Tracker),
		specFile: strings.TrimSpace(cfg.Project.SpecFile),
	}
	if e.mode == "" {
		e.mode = "advisory"
	}
	if strings.TrimSpace(cfg.Guardian.FlowFile) != "" {
		p, err := schema.Load(cfg.Guardian.FlowFile)
		if err != nil {
			e.policyErr = err
		} else {
			e.policy = p
		}
	}
	return e
}

func (e *PolicyEvaluator) Evaluate(_ context.Context, req Request) (Decision, error) {
	if e.policyErr != nil {
		return e.resolveFailure("guardian_policy_load_error", fmt.Sprintf("guardian policy load failed: %v", e.policyErr), "policy", ""), nil
	}
	if e.policy == nil {
		return Decision{Result: ResultAllow}, nil
	}
	ep := string(req.Event)
	best := Decision{Result: ResultAllow}
	for _, rule := range e.policy.Rules {
		if !rule.Enabled || !contains(rule.EnforcementPoints, ep) {
			continue
		}
		dec := e.evaluateRule(rule, req)
		if rank(dec.Result) > rank(best.Result) {
			best = dec
		}
	}
	return best, nil
}

func (e *PolicyEvaluator) evaluateRule(rule schema.Rule, req Request) Decision {
	switch rule.ID {
	case "ticket_desc_has_scope_and_verify":
		return e.evalTicketScopeVerify(rule, req)
	case "phase_has_int_gap_tst_chain":
		return e.evalPhaseChain(rule, req)
	case "prd_has_required_code_examples":
		return e.evalPRDExamples(rule)
	case "spec_has_api_and_schema_examples":
		return e.evalSpecExamples(rule)
	default:
		return Decision{Result: ResultAllow}
	}
}

func (e *PolicyEvaluator) evalTicketScopeVerify(rule schema.Rule, req Request) Decision {
	desc := strings.TrimSpace(asString(req.Context["desc"]))
	verifyCmd := strings.TrimSpace(asString(req.Context["verify_cmd"]))
	if !rules.TicketDescHasScopeAndVerify(desc) {
		return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "ticket missing scope and/or verify"), fmt.Sprintf("ticket:%s", strings.TrimSpace(req.TicketID)), "")
	}
	if verifyCmd == "" {
		return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "ticket verify command missing"), fmt.Sprintf("ticket:%s", strings.TrimSpace(req.TicketID)), "")
	}
	return Decision{Result: ResultAllow}
}

func (e *PolicyEvaluator) evalPhaseChain(rule schema.Rule, req Request) Decision {
	tickets, err := e.extractTickets(req)
	if err != nil {
		return e.resolveFailure(rule.ID, err.Error(), "phase", "")
	}
	phases := phaseNumbers(tickets)
	for _, phase := range phases {
		if req.Phase > 0 && phase != req.Phase {
			continue
		}
		res := rules.CheckPhaseIntGapTstChain(tickets, phase)
		if !res.Valid() {
			reason := coalesce(rule.FailReason, "phase missing int->gap->tst chain")
			if len(res.MissingKinds) > 0 {
				reason = fmt.Sprintf("%s (missing kinds: %s)", reason, strings.Join(res.MissingKinds, ","))
			} else if len(res.GapWithoutInt) > 0 {
				reason = fmt.Sprintf("%s (gap without int dep: %s)", reason, strings.Join(res.GapWithoutInt, ","))
			} else if len(res.TstWithoutGap) > 0 {
				reason = fmt.Sprintf("%s (tst without gap dep: %s)", reason, strings.Join(res.TstWithoutGap, ","))
			}
			return e.resolveFailure(rule.ID, reason, fmt.Sprintf("phase:%d", phase), "")
		}
	}
	return Decision{Result: ResultAllow}
}

func (e *PolicyEvaluator) evalPRDExamples(rule schema.Rule) Decision {
	paths, err := e.findMarkdownTargets([]string{"docs/prd/*.md", "PRD.md"})
	if err != nil {
		return e.resolveFailure(rule.ID, err.Error(), "prd", "")
	}
	if len(paths) == 0 {
		return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "no PRD files found"), "prd", "")
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(content)
		if hasHeadings(text, []string{"## Objective", "## Scope", "## Acceptance Criteria"}) && fencedCodeBlocks(text) >= 2 {
			return Decision{Result: ResultAllow}
		}
	}
	return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "PRD missing required headings or code examples"), "prd", "")
}

func (e *PolicyEvaluator) evalSpecExamples(rule schema.Rule) Decision {
	paths, err := e.findMarkdownTargets([]string{"docs/**/*SPEC*.md", "**/*BUILD-SPEC*.md"})
	if err != nil {
		return e.resolveFailure(rule.ID, err.Error(), "spec", "")
	}
	if len(paths) == 0 {
		return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "no spec files found"), "spec", "")
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.ToLower(string(content))
		apiFound := strings.Contains(text, "curl") || strings.Contains(text, "http") || strings.Contains(text, "endpoint")
		schemaFound := strings.Contains(text, "schema") || strings.Contains(text, "yaml") || strings.Contains(text, "json")
		if apiFound && schemaFound && fencedCodeBlocks(string(content)) >= 2 {
			return Decision{Result: ResultAllow}
		}
	}
	return e.resolveFailure(rule.ID, coalesce(rule.FailReason, "spec missing API/schema examples"), "spec", "")
}

func (e *PolicyEvaluator) resolveFailure(ruleID, reason, target, evidence string) Decision {
	res := ResultWarn
	if e.mode == "enforce" {
		res = ResultBlock
	}
	return Decision{
		Result:       res,
		RuleID:       strings.TrimSpace(ruleID),
		Reason:       strings.TrimSpace(reason),
		Target:       strings.TrimSpace(target),
		EvidencePath: strings.TrimSpace(evidence),
	}
}

func (e *PolicyEvaluator) extractTickets(req Request) (map[string]tracker.Ticket, error) {
	if v, ok := req.Context["tickets"].(map[string]tracker.Ticket); ok {
		return v, nil
	}
	if strings.TrimSpace(e.tracker) == "" {
		return nil, fmt.Errorf("tracker path not configured")
	}
	tr, err := tracker.Load(e.tracker)
	if err != nil {
		return nil, err
	}
	return tr.Tickets, nil
}

func (e *PolicyEvaluator) findMarkdownTargets(patterns []string) ([]string, error) {
	root := strings.TrimSpace(e.repoRoot)
	if root == "" {
		root = "."
	}
	all := make([]string, 0, 128)
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			all = append(all, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	matched := make(map[string]struct{})
	for _, p := range patterns {
		rx, err := compileGlob(strings.ToLower(filepath.ToSlash(strings.TrimSpace(p))))
		if err != nil {
			continue
		}
		for _, path := range all {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				continue
			}
			rel = strings.ToLower(filepath.ToSlash(rel))
			if rx.MatchString(rel) {
				matched[path] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(matched))
	for path := range matched {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteString("\\")
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func rank(r Result) int {
	switch r {
	case ResultBlock:
		return 3
	case ResultWarn:
		return 2
	default:
		return 1
	}
}

func hasHeadings(text string, headings []string) bool {
	lower := strings.ToLower(text)
	for _, h := range headings {
		if !strings.Contains(lower, strings.ToLower(h)) {
			return false
		}
	}
	return true
}

func fencedCodeBlocks(text string) int {
	count := strings.Count(text, "```")
	return count / 2
}

func phaseNumbers(tickets map[string]tracker.Ticket) []int {
	seen := map[int]struct{}{}
	for _, tk := range tickets {
		if tk.Phase > 0 {
			seen[tk.Phase] = struct{}{}
		}
	}
	phases := make([]int, 0, len(seen))
	for phase := range seen {
		phases = append(phases, phase)
	}
	sort.Ints(phases)
	return phases
}

func coalesce(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(fallback)
}
