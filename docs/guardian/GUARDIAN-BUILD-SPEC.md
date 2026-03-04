# Guardian Build Spec (Authoritative)

## Objective
Implement a configurable **Flow Guardian** for agent-swarm that enforces end-to-end project process compliance beyond DAG ordering.

## Problem
Current DAG enforces ticket order/dependencies/phase gates, but does not guarantee:
- PRD/spec quality
- artifact completeness
- approval hygiene
- ticket quality standards
- repeatable process evidence

## Target Outcome
A project-local, externally configurable guardian that can run in:
- `advisory` mode (warn only)
- `enforce` mode (hard blocks on unmet policy)

with machine-readable evidence and approval records.

## Scope (in)
1. Parse + validate `swarm/flow.v2.yaml`
2. Guardian evaluator (`ALLOW/WARN/BLOCK`)
3. Hook enforcement at:
   - phase transition (`go`, `post_build -> complete`)
   - before spawn
   - before mark-done
4. Evidence + approval stores in state dir
5. CLI surface for validate/check/report
6. Init scaffolding for default flow policy
7. Migration-safe rollout for existing projects

## Scope (out)
- Full custom rule scripting engine
- Multi-node distributed coordination
- UI/TUI guardian editor

## Functional Requirements

### FR1 — Externalized policy
- Flow policy is loaded from project-local file (`swarm/flow.v2.yaml` by default).
- Configurable via `swarm.toml`:

```toml
[guardian]
enabled = true
flow_file = "swarm/flow.v2.yaml"
mode = "advisory"   # advisory | enforce
```

### FR2 — Policy evaluation
- Evaluator returns one of: `ALLOW`, `WARN`, `BLOCK`
- Decision includes:
  - rule id
  - reason
  - target (ticket/state)
  - evidence reference

### FR3 — Enforcement points
- `before_spawn`
- `before_mark_done`
- `transition gates` (`go`, feature state transitions)
- `post_build -> complete`

### FR4 — Evidence model
Write to state dir:
- `guardian-events.jsonl`
- `approvals.json`
- `evidence/*` command outputs + artifact assertions

### FR5 — Mandatory quality rules
Must support blocking rules:
- `prd_has_required_code_examples`
- `spec_has_api_and_schema_examples`
- `ticket_desc_has_scope_and_verify`
- `phase_has_int_gap_tst_chain`

### FR6 — Backward compatibility
- Existing projects can run with guardian disabled or advisory
- No breaking behavior unless `mode=enforce`

## Non-functional Requirements
- Deterministic rule evaluation
- Fast checks on watch loop (cache expensive checks where possible)
- Human-readable block reasons
- Stable behavior across `watch --once` runs

## Architecture

### Components
1. `internal/guardian/schema` — YAML schema parser + validator
2. `internal/guardian/engine` — policy evaluator + decision model
3. `internal/guardian/evidence` — writes event/evidence/approval files
4. `internal/guardian/rules` — built-in rule implementations
5. `cmd/guardian.go` — CLI commands

### Integrations
- `watchdog.RunOnce` invokes guardian at enforcement points
- `go` command checks transition compliance
- `init` scaffolds default flow file

## Data Contracts

### Decision
```json
{
  "result": "ALLOW|WARN|BLOCK",
  "rule": "ticket_desc_has_scope_and_verify",
  "reason": "ticket missing verify command",
  "target": "ticket:ph2-04",
  "evidence": "~/.local/state/.../evidence/ticket-ph2-04.json"
}
```

### approvals.json
```json
{
  "prd_approved": {
    "by": "human",
    "at": "2026-03-04T08:00:00Z",
    "note": "approved"
  }
}
```

## Acceptance Criteria
1. `agent-swarm guardian validate` passes on default flow file
2. Missing required PRD code examples blocks PRD approval in enforce mode
3. Missing spec API/schema examples blocks spec approval in enforce mode
4. Missing ticket scope/verify blocks spawn in enforce mode
5. Missing int/gap/tst chain blocks planning transition in enforce mode
6. `guardian-events.jsonl` written with decisions + reasons
7. Existing projects run unchanged in advisory mode

## Rollout Plan
1. Build with default `mode=advisory`
2. Pilot one project for one week
3. tune noisy rules
4. switch pilot to enforce
5. roll out globally
