# SWARM Ticket Prep V2 Spec (Implementation-Ready Prompt Compiler)

Status: Proposed (ready to implement)
Owner: agent-swarm
Date: 2026-03-04

---

## 1) Problem Statement

Current `agent-swarm prompts gen <ticket>` creates generic boilerplate prompts that are not implementation-ready. This causes sub-agents to start with insufficient context and fail or produce low-signal output.

We need a deterministic ticket-prep pipeline where tickets are structured, prompts are compiled (not templated), and spawn is blocked unless all quality gates pass.

---

## 2) Goals

1. Every spawnable ticket is implementation-ready.
2. Prompt generation is deterministic and auditable.
3. Role/rules context is enforced and reproducible per sub-agent.
4. Preflight gates prevent low-quality swarm runs.
5. Post-build review steps run once per swarm run (not per feature).

---

## 3) Non-Goals

- Free-form AI prompt authoring without schema.
- Runtime prompt mutation during agent execution.
- Dynamic remote policy/rules fetch during spawn.

---

## 4) CLI Contract (New/Changed)

## 4.1 New commands

- `agent-swarm tickets lint [--strict] [--json]`
- `agent-swarm prompts build <ticket|--all> [--strict] [--dry-run]`
- `agent-swarm prompts validate [--strict] [--json] [--ticket <id>]`
- `agent-swarm prep [--strict] [--json]`
- `agent-swarm roles check [--json]`
- `agent-swarm migrate post-build-run-scope [--dry-run|--apply]`

## 4.2 Changed commands

- `agent-swarm prompts gen <ticket>` removed. Use `agent-swarm prompts build` only.
- `agent-swarm init <project>`
  - add `--with-agentkit` (default: true)
  - scaffolds `.agents/*` and `.codex/rules/*` by default.
- `agent-swarm watch` / `watch --once`
  - must fail fast unless `prep` checks pass (or `--allow-unprepared` explicit override).

Exit codes:
- 0: success
- 2: validation/preflight failure
- 3: spawn blocked by guardian policy
- 4: migration conflict or unsupported legacy state

---

## 5) Ticket Schema v2 (Required Fields)

Each ticket MUST include structured fields (not only `desc`).
If any required field is missing/invalid, ticket is not spawnable.

```json
{
  "id": "g1-01",
  "status": "todo",
  "phase": 1,
  "type": "feature",
  "runId": "run-2026-03-04T10-32-00Z",
  "role": "backend",
  "desc": "Guardian schema parser + validation",

  "objective": "Implement flow.v2 schema loader and validator with structured errors.",
  "scope_in": [
    "internal/guardian/schema/types.go",
    "internal/guardian/schema/load.go",
    "internal/guardian/schema/validate.go",
    "internal/guardian/schema/*_test.go"
  ],
  "scope_out": [
    "No changes to watchdog runtime behavior",
    "No CLI surface changes beyond schema package"
  ],
  "files_to_touch": [
    "internal/guardian/schema/*.go"
  ],
  "reference_files": [
    "internal/config/config.go",
    "cmd/prompts.go"
  ],
  "implementation_steps": [
    "Define policy structs",
    "Implement YAML loader",
    "Implement validator with multi-error output",
    "Add pass/fail tests"
  ],
  "tests_to_add_or_update": [
    "internal/guardian/schema/validate_test.go"
  ],
  "verify_cmd": "go test ./internal/guardian/schema/...",
  "acceptance_criteria": [
    "Valid policy loads successfully",
    "Invalid policy returns structured validation errors",
    "Tests pass"
  ],
  "constraints": [
    "No new runtime deps beyond yaml parser",
    "No tracker format changes in this ticket"
  ]
}
```

Validation rules:
- `objective`: non-empty, single sentence preferred.
- `scope_in`: non-empty array.
- `scope_out`: non-empty array.
- `files_to_touch`: non-empty, path-like patterns.
- `implementation_steps`: min 2 entries.
- `tests_to_add_or_update`: non-empty.
- `verify_cmd`: non-empty shell command.
- `acceptance_criteria`: non-empty; each criterion must be binary testable.
- `constraints`: non-empty.
- `role`: must resolve to existing role spec/profile/rules.

---

## 6) Prompt Compiler (`prompts build`)

`prompts build` is deterministic and fail-closed in strict mode.

Compiler inputs:
1. Ticket schema v2 fields
2. Project policy/standards from config
3. Role stack (base rules + role rules + role profile + role spec + skills)
4. Verify contract

Compiler output:
- `swarm/prompts/<ticket>.md` final execution prompt
- `swarm/prompts/<ticket>.manifest.json` compile manifest

Prompt deterministic section order:
1. Header (ticket id/title/type/phase/run)
2. Objective
3. Scope In
4. Scope Out
5. Files to Touch (exact)
6. Reference Files
7. Implementation Steps
8. Tests to Add/Update
9. Verify Command
10. Acceptance Criteria
11. Constraints
12. Commit Contract (what constitutes done)
13. Forbidden Actions
14. Agent Context Pointers

Strict failure conditions:
- Any unresolved placeholders (`TODO`, `<...>`, `TBD`, `Add details here`)
- Missing required sections
- Missing/invalid verify command
- Empty files_to_touch
- Role context unresolved

---

## 7) AgentKit Scaffolding (init defaults)

When `agent-swarm init` runs with `--with-agentkit=true` (default):

Created structure:

```
.agents/
  profiles/
    architect.md
    backend.md
    reviewer.md
    qa.md
  roles/
    architect.yaml
    backend.yaml
    reviewer.yaml
    qa.yaml
  skills/
    testing.md
    migration.md
    docs.md

.codex/
  rules/
    base.md
    architect.md
    backend.md
    reviewer.md
    qa.md
```

`roles check` verifies:
- ticket roles all resolve
- required role files exist
- required base rules exist
- no unresolved role references

---

## 8) Sub-agent Context Binding (copy-on-spawn)

Before spawning a ticket, materialize immutable context in the ticket worktree:

```
<worktree>/.agent-context/
  rules/base.md
  rules/<role>.md
  profiles/<role>.md
  roles/<role>.yaml
  skills/*.md
  context-manifest.json
```

`context-manifest.json`:

```json
{
  "ticket": "g1-01",
  "runId": "run-2026-03-04T10-32-00Z",
  "role": "backend",
  "createdAt": "2026-03-04T10:33:00Z",
  "sources": [
    {"path": ".codex/rules/base.md", "sha256": "..."},
    {"path": ".codex/rules/backend.md", "sha256": "..."},
    {"path": ".agents/profiles/backend.md", "sha256": "..."},
    {"path": ".agents/roles/backend.yaml", "sha256": "..."}
  ]
}
```

Tracker/events should log `context_manifest_path` for auditability.

---

## 9) Preflight Gate (`prep`)

`agent-swarm prep` runs before watch:

1. `tickets lint` (schema completeness/quality)
2. `prompts build --all`
3. `prompts validate --strict`
4. spawnability check (deps/phase/status/prompt readiness/role context)
5. post-build run-scope validation

`prep` must fail if:
- per-feature tickets contain legacy post-build deps
- run-level post-build config missing
- ticket type/runId mismatch
- role context unresolved
- prompt strict checks fail

Watch behavior:
- default: block `watch` if prep failed
- override: `--allow-unprepared` explicit and noisy warning

---

## 10) Guardian Integration

Add guardian rules (enforced at `before_spawn` in enforce mode):
- `ticket_has_required_fields`
- `prompt_has_required_sections`
- `prompt_has_verify_command`
- `prompt_has_explicit_file_scope`

Expected outcomes:
- advisory mode: WARN events only
- enforce mode: BLOCK spawn

Evidence paths:
- `evidence/ticket_has_required_fields-<ticket>.json`
- `evidence/prompt_has_required_sections-<ticket>.json`
- `evidence/prompt_has_verify_command-<ticket>.json`
- `evidence/prompt_has_explicit_file_scope-<ticket>.json`

---

## 11) Post-Build Once-Per-Run Model

### 11.1 Run-level state

Tracker includes:
- `currentRunId`
- `runs[runId].integration`
- `runs[runId].postBuild[step]`

### 11.2 Ticket types
- `feature`
- `integration`
- `post_build`

### 11.3 Dispatch order
1. Spawn feature tickets (phase-respecting)
2. On feature completion for run: spawn integration ticket
3. After integration done: spawn post-build steps (once each)
4. Mark run complete

Hard idempotency rule:
- key `<run_id>:<post_build_step>`
- if already done: never respawn

Default post-build order:
1. code-reviewer
2. database-reviewer
3. security-reviewer
4. doc-updater
5. refactor-cleaner

Config:

```toml
[post_build]
run_once_per_run = true
order = ["code-reviewer", "database-reviewer", "security-reviewer", "doc-updater", "refactor-cleaner"]
parallel_groups = []
max_retries = 1
```

---

## 12) Authoring Workflow (Planning → Execution)

1. Create structured tickets (schema v2)
2. Assign roles
3. `agent-swarm tickets lint --strict`
4. `agent-swarm prompts build --all --strict`
5. `agent-swarm prep --strict`
6. Human review only failed lint/build/prep outputs
7. Start swarm (`watch --once`/`watch`)

This preserves speed while ensuring implementation readiness.

---

## 13) Migration Plan

### 13.1 Prompt generation migration
- remove `prompts gen` entirely
- introduce warning: deprecated for execution
- docs/examples switch to `prompts build`

### 13.2 Tracker migration
`agent-swarm migrate post-build-run-scope`:
- detect legacy per-feature post-build deps
- create run-level post-build states
- remove duplicated per-feature post-build dependencies
- support `--dry-run` diff and `--apply`

---

## 14) Acceptance Criteria (System-level)

1. `tickets lint --strict` fails on missing required structured fields.
2. `prompts build --all --strict` generates deterministic prompts with no placeholders.
3. `prep --strict` blocks watch when prompt/schema/role context is invalid.
4. Spawn in enforce mode is blocked by guardian if ticket/prompt checks fail.
5. Post-build reviewers run once per run (never per feature).
6. Each spawned ticket has `.agent-context/context-manifest.json` with source hashes.
7. Status/API clearly separates feature progress from run-level post-build progress.

---

## 15) Implementation Backlog (suggested)

- `tp-01` tracker schema v2 + validators
- `tp-02` `tickets lint` command
- `tp-03` prompt compiler (`prompts build`) + manifest output
- `tp-04` `prompts validate --strict`
- `tp-05` `prep` command and watch gate
- `tp-06` init agentkit scaffolding (`.agents`, `.codex/rules`)
- `tp-07` roles resolver + `roles check`
- `tp-08` copy-on-spawn `.agent-context` + manifest hashes
- `tp-09` guardian rule implementations (before_spawn checks)
- `tp-10` run-scope post-build state + dispatcher idempotency
- `tp-11` migration command for legacy trackers
- `tp-12` TUI/API updates for run-level post-build reporting

