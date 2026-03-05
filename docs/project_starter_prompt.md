# Project Starter Prompt Template

Use this structure when kicking off a new project build with Navi. The more concrete and specific the inputs, the more accurate the implementation.

---

## Template

```markdown
# Project: [NAME]

## 1. One-liner
[What is this in one sentence?]

## 2. Problem
[What's broken/missing today? Be specific about the pain point.]

## 3. Target Outcome
[What does success look like? Observable behavior, not just "it works."]

## 4. Scope

### In scope
- [Deliverable 1] — [location hint: CLI command / internal package / API endpoint]
- [Deliverable 2] — [location hint]
- [Deliverable 3] — [location hint]

### Out of scope
- [What we're explicitly NOT building]
- [Another thing to avoid]

## 5. Technical Constraints
- Language/framework: [e.g., "Go CLI using cobra"]
- Must integrate with: [existing system X]
- Must not break: [existing behavior Y]
- Performance: [if relevant]
- Security: [if relevant]

## 6. Architecture Sketch

[ASCII diagram or short description of component relationships]

```
component1 → component2 → component3
     ↓
  storage
```

[One-paragraph description of data flow]

## 7. Data Contracts

### Input example
```json
{
  "field": "value"
}
```

### Output example
```json
{
  "result": "value"
}
```

### Config example (if applicable)
```toml
[section]
key = "value"
```

## 8. Reference Implementation Pattern
[Point to ONE existing file that shows the pattern to follow]

- "See `path/to/existing/file.go` for how we do X"
- "Follow the structure in `cmd/status.go` for CLI commands"

## 9. Verification Criteria
[For each deliverable, a command that PROVES it exists and works]

- `ls path/to/expected/` — directory exists
- `go build ./cmd/...` — compiles without error
- `<binary> <command> --help` — command registered
- `go test ./path/to/... -v` — tests pass

## 10. Acceptance Scenarios
[Concrete test cases that define "done"]

1. Given [precondition], when [action], then [expected result]
2. Given [precondition], when [action], then [expected result]
3. Given [precondition], when [action], then [expected result]

## 11. Non-goals / Deferred
[Explicitly punted to v2/later — prevents scope creep]

- [Feature X] — deferred to v2
- [Optimization Y] — not needed for MVP

## 12. Open Questions
[Things you're unsure about — I'll ask before building]

- [Question 1]
- [Question 2]
```

---

## Concrete Example: Guardian (Corrected)

Here's how the Guardian project spec SHOULD have looked to avoid ghost completions:

```markdown
# Project: Flow Guardian

## 1. One-liner
Add configurable process/policy enforcement to agent-swarm beyond DAG ordering.

## 2. Problem
Current agent-swarm enforces ticket order via DAG but doesn't guarantee:
- PRD/spec quality (could be empty files)
- Required artifacts exist before transitions
- Approval hygiene (who approved what, when)
- Ticket quality standards (scope, verify commands)

Agents mark tickets "done" without verification that deliverables actually exist.

## 3. Target Outcome
A guardian subsystem that:
- Runs in `advisory` mode (logs warnings) or `enforce` mode (blocks transitions)
- Validates policy rules at spawn, mark-done, and phase transitions
- Writes machine-readable evidence to state directory
- Adds CLI commands: `guardian validate`, `guardian check`, `guardian report`

Observable: `agent-swarm guardian validate` returns exit 0 on valid policy, exit 1 with structured errors on invalid.

## 4. Scope

### In scope
- `internal/guardian/schema/` — YAML policy parser + validator
- `internal/guardian/engine/` — policy evaluator returning ALLOW/WARN/BLOCK
- `internal/guardian/evidence/` — writes events + approvals to state dir
- `internal/guardian/rules/` — built-in rule implementations
- `cmd/guardian.go` — CLI subcommands
- `swarm/flow.v2.yaml` — default policy file (embedded in binary)
- `[guardian]` config section in swarm.toml

### Out of scope
- Custom rule scripting engine
- UI/TUI guardian editor
- Multi-node coordination

## 5. Technical Constraints
- Language: Go (same as agent-swarm)
- CLI framework: cobra (same as existing commands)
- Must integrate with: existing watchdog spawn/done paths
- Must not break: existing projects with no guardian config (backward compatible)
- Config: TOML for swarm.toml, YAML for policy files

## 6. Architecture Sketch

```
swarm.toml [guardian] config
         ↓
    config.Load()
         ↓
  guardian/schema.Parse(flow.v2.yaml)
         ↓
  guardian/engine.Evaluate(rule, context)
         ↓
  ALLOW / WARN / BLOCK decision
         ↓
  guardian/evidence.Write(event)
         ↓
  ~/.local/state/agent-swarm/<project>/guardian-events.jsonl
```

Watchdog calls `engine.Evaluate()` at:
- `before_spawn` — can block ticket spawn
- `before_mark_done` — can block completion
- `phase_transition` — can block `go` command

## 7. Data Contracts

### Policy file (flow.v2.yaml)
```yaml
version: 2
mode: advisory  # or "enforce"

rules:
  - id: prd_has_code_examples
    enabled: true
    enforcement_points: [transition]
    
  - id: ticket_has_verify_cmd
    enabled: true
    enforcement_points: [before_spawn]
```

### Decision struct (Go)
```go
type Decision struct {
    Result   string `json:"result"`   // "ALLOW", "WARN", "BLOCK"
    Rule     string `json:"rule"`     // rule id
    Reason   string `json:"reason"`   // human-readable
    Target   string `json:"target"`   // "ticket:ph2-04" or "transition:go"
    Evidence string `json:"evidence"` // path to evidence file
}
```

### Event (guardian-events.jsonl)
```json
{"ts":"2026-03-05T10:00:00Z","rule":"ticket_has_verify_cmd","result":"BLOCK","target":"ticket:g1-01","reason":"missing verify_cmd field"}
```

### Config section (swarm.toml)
```toml
[guardian]
enabled = true
flow_file = "swarm/flow.v2.yaml"
mode = "advisory"
```

## 8. Reference Implementation Pattern

- Config parsing: `internal/config/config.go` lines 20-80 (struct + Load function)
- CLI subcommand: `cmd/status.go` (cobra command structure)
- YAML parsing: use `gopkg.in/yaml.v3` (already in go.mod)
- Event writing: `internal/watchdog/watchdog.go` `appendEvent()` function

## 9. Verification Criteria

Each ticket must pass ALL of these to be marked done:

### Directory/file existence
```bash
ls internal/guardian/schema/    # must exist
ls internal/guardian/engine/    # must exist
ls internal/guardian/evidence/  # must exist
ls internal/guardian/rules/     # must exist
ls cmd/guardian.go              # must exist
```

### Compilation
```bash
go build ./...                  # zero errors
```

### CLI registration
```bash
agent-swarm guardian --help     # shows subcommands
agent-swarm guardian validate --help  # shows flags
```

### Tests exist and pass
```bash
go test ./internal/guardian/... -v    # tests exist and pass
go test ./cmd/... -run Guardian -v    # CLI tests pass
```

### Integration check
```bash
# Create test policy, run validate
echo 'version: 2
mode: advisory
rules: []' > /tmp/test-flow.yaml

agent-swarm guardian validate --flow /tmp/test-flow.yaml
echo $?  # must be 0
```

## 10. Acceptance Scenarios

1. **Valid policy passes validation**
   - Given: a syntactically correct flow.v2.yaml
   - When: `agent-swarm guardian validate` runs
   - Then: exit 0, no output

2. **Invalid policy fails validation**
   - Given: flow.v2.yaml with unknown rule id
   - When: `agent-swarm guardian validate` runs
   - Then: exit 1, JSON error listing unknown rule

3. **Advisory mode logs but doesn't block**
   - Given: `mode: advisory` in config
   - When: ticket spawned without verify_cmd
   - Then: warning logged to guardian-events.jsonl, spawn proceeds

4. **Enforce mode blocks**
   - Given: `mode: enforce` in config
   - When: ticket spawned without verify_cmd
   - Then: spawn blocked, error returned, event logged

5. **Backward compatible**
   - Given: swarm.toml with no [guardian] section
   - When: `agent-swarm watch` runs
   - Then: works exactly as before, no guardian checks

6. **Report command summarizes**
   - Given: guardian-events.jsonl with 3 BLOCK, 2 WARN events
   - When: `agent-swarm guardian report` runs
   - Then: outputs summary table with counts by rule

## 11. Non-goals / Deferred

- **Custom rule scripting** — v2; for now, only built-in rules
- **Approval workflows** — v2; for now, just record approvals.json
- **TUI integration** — v2; CLI-only for now
- **Remote policy fetch** — v2; policy must be local file

## 12. Open Questions

None — spec is complete. Ready to build.
```

---

## Why This Version Would Have Worked

1. **Existence verification** — `ls internal/guardian/schema/` fails if directory doesn't exist. Previous spec only had `go test` which passes on empty.

2. **Concrete Go types** — `type Decision struct { ... }` shows exact field names and types. No ambiguity.

3. **Reference files** — "See `internal/config/config.go` lines 20-80" gives agents a concrete anchor.

4. **CLI verification** — `agent-swarm guardian --help` proves the command is registered, not just that code compiles.

5. **Integration check script** — Runnable end-to-end test that proves the feature works.

6. **Acceptance scenarios with Given/When/Then** — Unambiguous test cases that define done.

---

## Minimal Version (for small features)

```markdown
# Feature: [name]

**Problem:** [1-2 sentences]

**Solution:** [1-2 sentences]

**Files to create/modify:**
- `path/to/file.go` — [what it does]
- `path/to/other.go` — [what it does]

**Reference pattern:** `path/to/similar/existing.go`

**Verify:**
- `ls path/to/new/file.go` — file exists
- `go build ./...` passes
- `go test ./path/to/... -v` passes

**Acceptance:**
1. When I run [X], I get [Y]
2. When I run [A] with [B], I get [C]
```

---

## Checklist Before Submitting

- [ ] One-liner is actually one sentence
- [ ] Problem describes pain, not solution
- [ ] Scope has explicit IN and OUT lists
- [ ] At least one reference file cited
- [ ] Verification includes existence checks (ls/stat), not just tests
- [ ] Acceptance scenarios use Given/When/Then format
- [ ] Data contracts show actual JSON/YAML/Go, not prose descriptions
- [ ] Open questions are empty OR explicitly listed
