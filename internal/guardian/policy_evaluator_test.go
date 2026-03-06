package guardian

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func writePolicy(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "flow.v2.yaml")
	policy := `version: 2
mode: advisory
settings:
  fail_closed: false
  cache_ttl_seconds: 0
  max_evidence_bytes: 1024
enforcement_points: [before_spawn, transition]
contexts:
  default:
    severity: block
rules:
  - id: ticket_desc_has_scope_and_verify
    enabled: true
    description: test
    severity: block
    enforcement_points: [before_spawn]
    target:
      kind: ticket
      source: swarm/tracker.json
      fields: [scope, verify]
    check:
      type: ticket_fields
      params: {}
    pass_when:
      op: all
      conditions:
        - metric: required_fields_present
          equals: true
    fail_reason: bad ticket
    evidence:
      kind: json
      path: evidence/ticket.json
  - id: phase_has_int_gap_tst_chain
    enabled: true
    description: test
    severity: block
    enforcement_points: [transition]
    target:
      kind: phase
      source: swarm/tracker.json
    check:
      type: phase_chain
      params: {}
    pass_when:
      op: all
      conditions:
        - metric: required_kinds_present
          equals: true
    fail_reason: bad phase
    evidence:
      kind: json
      path: evidence/phase.json
overrides:
  enabled: false
  require_reason: false
  require_expiry: false
  max_duration_hours: 0
  store: approvals.json
events:
  file: guardian-events.jsonl
  include: [timestamp]
`
	if err := os.WriteFile(path, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

func TestPolicyEvaluatorAdvisoryWarnsOnTicketRule(t *testing.T) {
	dir := t.TempDir()
	flow := writePolicy(t, dir)
	cfg := &config.Config{}
	cfg.Project.Repo = dir
	cfg.Guardian.FlowFile = flow
	cfg.Guardian.Mode = "advisory"

	ev := NewPolicyEvaluator(cfg)
	dec, err := ev.Evaluate(t.Context(), Request{
		Event:    EventBeforeSpawn,
		TicketID: "g1",
		Context: map[string]any{
			"desc":       "missing required fields",
			"verify_cmd": "",
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if dec.Result != ResultWarn {
		t.Fatalf("result=%s want WARN", dec.Result)
	}
	if dec.RuleID != "ticket_desc_has_scope_and_verify" {
		t.Fatalf("rule=%q", dec.RuleID)
	}
}

func TestPolicyEvaluatorEnforceBlocksPhaseChain(t *testing.T) {
	dir := t.TempDir()
	flow := writePolicy(t, dir)
	cfg := &config.Config{}
	cfg.Project.Repo = dir
	cfg.Guardian.FlowFile = flow
	cfg.Guardian.Mode = "enforce"

	tickets := map[string]tracker.Ticket{
		"int-g5": {Phase: 3, Type: "int"},
		"tst-g5": {Phase: 3, Type: "tst", Depends: []string{"int-g5"}},
	}
	ev := NewPolicyEvaluator(cfg)
	dec, err := ev.Evaluate(t.Context(), Request{
		Event: EventPhaseTransition,
		Phase: 3,
		Context: map[string]any{
			"tickets": tickets,
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if dec.Result != ResultBlock {
		t.Fatalf("result=%s want BLOCK", dec.Result)
	}
	if dec.RuleID != "phase_has_int_gap_tst_chain" {
		t.Fatalf("rule=%q", dec.RuleID)
	}
}
