# Guardian Tickets

## Phase G1 — Schema + Advisory Engine

### g1-01
**Objective:** Implement `flow.v2.yaml` parser and schema validation.
**Scope:** `internal/guardian/schema/*`
**Depends:** none
**Done criteria:** invalid schema returns structured errors.
**Verify:** `go test ./internal/guardian/schema/...`

### g1-02
**Objective:** Implement decision model (`ALLOW/WARN/BLOCK`) and advisory evaluator.
**Scope:** `internal/guardian/engine/*`
**Depends:** g1-01
**Done criteria:** evaluator returns deterministic decisions for sample policies.
**Verify:** `go test ./internal/guardian/engine/...`

### g1-03
**Objective:** Emit guardian decisions to state `guardian-events.jsonl`.
**Scope:** `internal/guardian/evidence/*`
**Depends:** g1-02
**Done criteria:** every advisory check emits event with rule/reason/target.
**Verify:** `go test ./internal/guardian/evidence/...`

### int-g1
Integration for G1 modules.
Depends: g1-01,g1-02,g1-03

### tst-g1
Verification for G1:
- `agent-swarm guardian validate`
- `watch --once` emits advisory events without blocking
Depends: int-g1

---

## Phase G2 — Enforce Transition Gates

### g2-01
**Objective:** Hook guardian into phase transition (`go`) evaluation.
**Scope:** `cmd/go.go`, guardian integration layer
**Depends:** tst-g1
**Done criteria:** blocked transition returns explicit unmet conditions.
**Verify:** `go test ./cmd/... -run GuardianGo`

### g2-02
**Objective:** Enforce transition rules for `post_build -> complete`.
**Scope:** watchdog transition enforcement
**Depends:** g2-01
**Done criteria:** missing required artifacts/rules block completion.
**Verify:** `go test ./internal/watchdog/... -run GuardianTransition`

### g2-03
**Objective:** Implement required transition rules:
- prd_has_required_code_examples
- spec_has_api_and_schema_examples
**Scope:** `internal/guardian/rules/*`
**Depends:** g2-01
**Done criteria:** rule tests cover pass/fail cases.
**Verify:** `go test ./internal/guardian/rules/... -run PRD|SPEC`

### int-g2
Integration for G2 transition enforcement.
Depends: g2-01,g2-02,g2-03

### tst-g2
Transition enforcement verification in enforce mode.
Depends: int-g2

---

## Phase G3 — Spawn/Done Hook Enforcement

### g3-01
**Objective:** Enforce guardian checks `before_spawn`.
**Scope:** watchdog spawn path
**Depends:** tst-g2
**Done criteria:** spawn blocked on unmet policy.
**Verify:** `go test ./internal/watchdog/... -run GuardianSpawn`

### g3-02
**Objective:** Enforce guardian checks `before_mark_done`.
**Scope:** watchdog done path
**Depends:** g3-01
**Done criteria:** done blocked when required checks fail.
**Verify:** `go test ./internal/watchdog/... -run GuardianDone`

### g3-03
**Objective:** Implement rules:
- ticket_desc_has_scope_and_verify
- prompt template section checks
**Scope:** `internal/guardian/rules/*`
**Depends:** g3-01
**Done criteria:** rule tests cover positive/negative.
**Verify:** `go test ./internal/guardian/rules/... -run Ticket|Prompt`

### int-g3
Integration for spawn/done enforcement.
Depends: g3-01,g3-02,g3-03

### tst-g3
End-to-end spawn/done enforcement verification.
Depends: int-g3

---

## Phase G4 — Evidence + Approvals

### g4-01
**Objective:** Implement state approvals store (`approvals.json`).
**Scope:** `internal/guardian/evidence/approvals.go`
**Depends:** tst-g3
**Done criteria:** approvals can be set/read/audited.
**Verify:** `go test ./internal/guardian/evidence/... -run Approval`

### g4-02
**Objective:** Capture check evidence payloads in `evidence/*`.
**Scope:** guardian evidence writer + rule execution wrappers
**Depends:** g4-01
**Done criteria:** each block/warn has evidence reference.
**Verify:** `go test ./internal/guardian/... -run Evidence`

### g4-03
**Objective:** Add `agent-swarm guardian report` output.
**Scope:** `cmd/guardian.go`
**Depends:** g4-02
**Done criteria:** report summarizes failures, reasons, evidence paths.
**Verify:** `go test ./cmd/... -run GuardianReport`

### int-g4
Integration for approvals/evidence/report.
Depends: g4-01,g4-02,g4-03

### tst-g4
Verification of machine-readable evidence + reporting.
Depends: int-g4

---

## Phase G5 — Init + Migration + Defaults

### g5-01
**Objective:** Add `[guardian]` config parsing (enabled/flow_file/mode).
**Scope:** `internal/config/*`
**Depends:** tst-g4
**Done criteria:** defaults + overrides parsed and validated.
**Verify:** `go test ./internal/config/... -run Guardian`

### g5-02
**Objective:** `init` scaffolds default `swarm/flow.v2.yaml`.
**Scope:** `cmd/init.go`, embedded assets
**Depends:** g5-01
**Done criteria:** new projects include default guardian flow file.
**Verify:** `go test ./cmd/... -run Init`

### g5-03
**Objective:** Implement migration helper for existing projects (advisory-first).
**Scope:** `cmd/guardian_migrate.go`
**Depends:** g5-02
**Done criteria:** migration command writes safe defaults and dry-run report.
**Verify:** `go test ./cmd/... -run GuardianMigrate`

### g5-04
**Objective:** Implement `phase_has_int_gap_tst_chain` rule.
**Scope:** `internal/guardian/rules/*`
**Depends:** g5-01
**Done criteria:** rule validates required chain in planned ticket set.
**Verify:** `go test ./internal/guardian/rules/... -run Chain`

### int-g5
Final integration for config/init/migration/rules.
Depends: g5-01,g5-02,g5-03,g5-04

### tst-g5
Final guardian verification matrix:
- advisory mode (non-blocking)
- enforce mode (blocking)
- init + migrate flows
Depends: int-g5
