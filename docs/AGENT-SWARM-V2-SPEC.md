# agent-swarm v2 — Process Specification
_Version: 1.0 | Author: Navi | Date: 2026-03-03 | Status: AWAITING APPROVAL_

## 1. Overview

Transform `agent-swarm` from a ticket executor into a **feature-driven development pipeline** with deterministic gates, specialist agents, and full lifecycle management.

### Design Principles
1. **Tool enforces, Navi drives** — State machine and gates in Go code. Navi writes PRDs/specs and triggers transitions.
2. **Separate sessions per specialist** — Each agent profile runs in its own Codex tmux session with the right model.
3. **Always all 7 post-build tickets** — No exceptions, no tiers. Every feature gets the full pipeline.
4. **Auto-fix critical/high, report medium/low** — Review findings create fix tickets for critical/high severity. Medium/low reported to human.
5. **No external governance dependency** — TDD enforcement via prompt-footer + tool validation.

---

## 2. Feature Lifecycle

### 2.1 Directory Structure

```
<project>/
  swarm/
    swarm.toml
    tracker.json
    features/
      <feature-name>/
        prd.md              # Product Requirements Document
        spec.md             # Technical specification
        arch-review.md      # Architecture review output (from architect agent)
        review-report.md    # Code review findings
        sec-report.md       # Security review findings
        gap-report.md       # Gap assessment findings
    prompts/
      <ticket-id>.md        # Per-ticket prompts (auto-generated or manual)
    profiles/               # Symlinks or copies of agent profiles
```

### 2.2 State Machine

```
draft → prd_review → arch_review → spec_review → planned → building → post_build → complete
         ↑ human      ↑ agent        ↑ human       ↑ tool    ↑ agents   ↑ agents     ↑ human
```

| State | Entry Condition | Exit Condition | Actor |
|-------|----------------|----------------|-------|
| `draft` | `agent-swarm feature add` | PRD file exists | Navi |
| `prd_review` | PRD exists in feature dir | `agent-swarm feature approve-prd <name>` | Mike |
| `arch_review` | PRD approved | Architect agent completes, outputs `arch-review.md` | Agent (sonnet) |
| `spec_review` | Arch review done, spec.md exists | `agent-swarm feature approve-spec <name>` | Mike |
| `planned` | Spec approved | Tickets registered in tracker, all have prompts | Tool (auto) |
| `building` | All tickets have prompts, deps satisfied | All `feat-*` tickets done (with phase gates) | Agents (codex) + Mike (gates) |
| `post_build` | All build tickets done | All 8 post-build tickets done | Agents (mixed) |
| `complete` | All post-build done, no critical findings open | `agent-swarm feature complete <name>` | Mike |

**Building phase retains the existing phase gate system:**
- Build tickets are grouped into phases (as today)
- Watchdog auto-spawns within a phase
- Phase gate requires human approval (`agent-swarm tui` → `A` to approve) before next phase spawns
- `auto_approve` setting in swarm.toml still respected (true = skip gates, false = require approval)
- This is the existing behavior — feature state machine wraps around it, doesn't replace it

**Hard gates (tool-enforced):**
- Cannot enter `arch_review` without PRD approval
- Cannot enter `spec_review` without arch review output
- Cannot enter `planned` without spec approval
- Cannot spawn build tickets without prompts
- Cannot enter `post_build` without all build tickets done
- Cannot enter `complete` with open critical/high fix tickets

### 2.3 Feature Metadata

Stored in `swarm/features/<name>/feature.json`:
```json
{
  "name": "cache-overhaul",
  "state": "building",
  "prd_approved_at": "2026-03-03T04:00:00Z",
  "prd_approved_by": "mike",
  "arch_review_at": "2026-03-03T04:15:00Z",
  "spec_approved_at": "2026-03-03T04:30:00Z",
  "spec_approved_by": "mike",
  "tickets": ["feat-1", "feat-2", "feat-3"],
  "post_build_tickets": ["int-1", "gap-1", "tst-1", "review-1", "sec-1", "doc-1", "clean-1", "mem-1"],
  "fix_tickets": ["fix-1", "fix-2"]
}
```

---

## 3. Ticket Types & Agent Profiles

### 3.1 Profile → Model Mapping

| Profile | Model | Mode | Read-Only? |
|---------|-------|------|------------|
| `architect` | claude-opus-4-6 | Research | Yes — outputs ADR, never writes code |
| `planner` | claude-opus-4-6 | Research | Yes — outputs ticket breakdown |
| `code-agent` + `tdd-guide` | gpt-5.3-codex | Development | No — writes tests + code |
| `code-reviewer` | claude-sonnet-4-6 | Review | Yes — outputs findings JSON |
| `security-reviewer` | claude-sonnet-4-6 | Review | Yes — outputs findings JSON |
| `e2e-runner` | gpt-5.3-codex | Development | No — writes + runs tests |
| `doc-updater` | gpt-5.2 | Development | No — writes docs |
| `refactor-cleaner` | gpt-4.1 | Development | No — modifies code |
| `build-error-resolver` | gpt-5.3-codex | Development | No — fixes build errors |

### 3.2 Ticket Type System

Every feature generates these ticket types in order:

#### Pre-Build (auto-created on feature add)
| ID Pattern | Type | Profile | Purpose |
|------------|------|---------|---------|
| `arch-<feature>` | Architecture Review | `architect` | Review PRD, produce ADR with risks/tradeoffs |

#### Build (created during `planned` phase)
| ID Pattern | Type | Profile | Purpose |
|------------|------|---------|---------|
| `<feature>-<N>` | Feature Implementation | `code-agent` + `tdd-guide` | TDD: tests first, then implement |

#### Post-Build (auto-appended when all build tickets done)
| ID Pattern | Type | Profile | Purpose | Order |
|------------|------|---------|---------|-------|
| `int-<feature>` | Integration Merge | (generic codex) | Merge all feature branches to base, resolve conflicts | 1 |
| `gap-<feature>` | Gap Assessment | `code-reviewer` | Compare spec vs implementation, flag missing requirements | 2 |
| `tst-<feature>` | E2E Test + Build | `e2e-runner` | Full test suite, build verification, smoke test | 3 |
| `review-<feature>` | Code Review | `code-reviewer` | Severity-ranked findings (critical/high/medium/low) | 4 |
| `sec-<feature>` | Security Review | `security-reviewer` | OWASP checklist, secrets scan, auth gates | 5 |
| `doc-<feature>` | Documentation | `doc-updater` | Feature docs + technical docs | 6 |
| `clean-<feature>` | Refactor/Cleanup | `refactor-cleaner` | Dead code, duplication, consolidation | 7 |
| `mem-<feature>` | Lessons Learned | `code-reviewer` | Record patterns, pitfalls, decisions into MEMORY.md | 8 |

#### Fix (auto-created from review/security findings)
| ID Pattern | Type | Profile | Purpose |
|------------|------|---------|---------|
| `fix-<feature>-<N>` | Fix | `code-agent` | Fix critical/high findings from review/security |

### 3.3 Review Output Format

Review and security agents must output structured JSON:
```json
{
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "security|correctness|performance|style|documentation",
      "file": "src/routes/api.ts",
      "line": 42,
      "title": "SQL injection in user input",
      "description": "User input passed directly to query without sanitization",
      "suggested_fix": "Use parameterized query"
    }
  ],
  "verdict": "BLOCK|WARN|PASS",
  "summary": "2 critical, 1 high, 3 medium findings"
}
```

**Verdict rules:**
- Any `critical` → `BLOCK` (auto-creates fix tickets)
- Any `high` with no `critical` → `BLOCK` (auto-creates fix tickets)
- Only `medium`/`low` → `WARN` (reported to human, no auto-fix)
- No findings → `PASS`

---

## 4. Prompt Generation

### 4.1 Prompt Assembly

Every ticket prompt is assembled from layers:

```
[1] Project governance  — AGENTS.md + CODEX.md + MEMORY.md from project root (agent contract, coding rules, project memory)
[2] Project context     — spec_file content (SPEC.md, etc.)
[3] Feature context     — prd.md + spec.md + arch-review.md (if exists)
[4] Profile content     — agent profile markdown (architect.md, tdd-guide.md, etc.)
[5] Ticket description  — from tracker.json `desc` field
[6] Prompt footer       — mandatory TDD process (current prompt-footer.md, minus legacy governance preflight)
[7] Conflict zones      — list of directories/files other agents are touching (parallel safety)
```

**Layer 1 is mandatory.** Every agent gets the project's AGENTS.md (coding standards, git hygiene, golden rules), CODEX.md (Codex-specific instructions), and MEMORY.md (project decisions, patterns, known issues). This ensures every agent operates under the same contract and has project memory context.

### 4.2 Auto-Generation

When `agent-swarm watch` encounters a ticket with no prompt file:
1. Read ticket `desc` from tracker
2. Read `spec_file` from swarm.toml
3. Read feature's `prd.md` + `spec.md` if feature association exists
4. Look up ticket type → profile mapping
5. Read profile markdown
6. Assemble prompt from layers above
7. Save to `swarm/prompts/<ticket-id>.md`

### 4.3 Prompt Footer (Updated)

```markdown
---

## MANDATORY DEVELOPMENT PROCESS (follow in exact order)

### Phase 1: Understand the spec
- Read the task objective and requirements above
- Read the source file(s) — understand inputs, outputs, error cases
- Read an existing test for the project's mocking pattern

### Phase 2: Write tests FIRST
- Write failing tests that cover: happy path, error path, edge cases
- Mock external dependencies (DB, HTTP) — no real connections
- Run tests — they SHOULD fail (red)

### Phase 3: Implement
- Write minimum code to make tests pass
- For test-only tickets: skip this phase

### Phase 4: Quality gates (fix and re-run until ALL green)
1. Tests pass: run project test command
2. Build check: run project build command (best-effort, don't fix pre-existing errors)
3. Lint: run project lint command

### Phase 5: Commit and push
```bash
git add -A
git commit -m "<type>: <description>"
git push origin HEAD
```
Do NOT ask for permission. Do NOT exit without committing and pushing.
If tests/build fail and you cannot fix them, commit what you have with a
`wip:` prefix and push anyway — the orchestrator will handle it.
```

### 4.4 Review/Security Prompt Suffix

Appended to review and security tickets only:
```markdown
---

## OUTPUT FORMAT (mandatory)

You are a READ-ONLY reviewer. Do NOT modify any source files.

Output your findings as a JSON file at `swarm/features/<feature>/review-report.json`
(or `sec-report.json` for security reviews).

Use this exact schema:
{findings: [{severity, category, file, line, title, description, suggested_fix}], verdict, summary}

Severity levels: critical, high, medium, low
Verdict: BLOCK if any critical or high. WARN if only medium/low. PASS if clean.

After writing the JSON, commit and push it.
```

---

## 5. New CLI Commands

### 5.1 Feature Management

```bash
# Create a new feature (creates directory, sets state=draft)
agent-swarm feature add <name> [--prd <path>]

# Approve PRD (gate: PRD file must exist)
agent-swarm feature approve-prd <name>

# Spawn architect agent to review PRD (gate: PRD must be approved)
# Architect outputs arch-review.md, state advances automatically
agent-swarm feature arch-review <name>

# Approve spec (gate: spec.md must exist, arch review must be done)
agent-swarm feature approve-spec <name>

# Decompose spec into tickets (gate: spec must be approved)
# Reads spec, creates tickets in tracker, generates prompts
agent-swarm feature plan <name> [--tickets <path-to-tickets.json>]

# List features and their states
agent-swarm feature list

# Show feature detail (state, tickets, findings)
agent-swarm feature show <name>

# Mark feature complete (gate: all tickets done, no open critical/high)
agent-swarm feature complete <name>
```

### 5.2 Validation

```bash
# Validate swarm state
agent-swarm validate

# Checks:
# - All todo tickets have prompts (or can be auto-generated)
# - Dependency graph is acyclic
# - Phase assignments are consistent
# - All features have required files for their state
# - No orphan tickets (tickets without feature association)
# - Profile files exist for all ticket types
```

### 5.3 Project Init (updated)

```bash
agent-swarm init <project>
```

Creates a fully self-contained project scaffold:
```
<project>/
  AGENTS.md                    # Standard agent contract (embedded in binary)
  swarm.toml                   # Swarm configuration
  swarm/
    tracker.json               # Empty ticket tracker
    prompts/                   # Per-ticket prompts
    features/                  # Feature lifecycle directories
    logs/                      # Agent output logs
  .agents/
    skills/                    # ECC skills (56 skills from everything-claude-code)
    profiles/                  # Specialist agent profiles (10 profiles)
  .codex/
    rules/                     # Coding rules per language (common, golang, python, typescript, swift)
```

All assets are **embedded in the binary** via `go:embed` — no external dependencies.

### 5.4 Ticket ID Format

Ticket IDs use a short `<prefix>-<NN>` pattern:

| Type | Pattern | Example |
|------|---------|---------|
| Feature | `<feat>-NN` | `cch-01`, `cch-02` (cache feature) |
| Architecture Review | `arc-<feat>` | `arc-cch` |
| Integration | `int-<feat>` | `int-cch` |
| Gap Assessment | `gap-<feat>` | `gap-cch` |
| E2E Test | `tst-<feat>` | `tst-cch` |
| Code Review | `rev-<feat>` | `rev-cch` |
| Security Review | `sec-<feat>` | `sec-cch` |
| Documentation | `doc-<feat>` | `doc-cch` |
| Cleanup | `cln-<feat>` | `cln-cch` |
| Lessons Learned | `mem-<feat>` | `mem-cch` |
| Fix | `fix-<feat>-NN` | `fix-cch-01` |

Feature prefix is a 3-letter abbreviation chosen at `feature add` time.

### 5.5 Existing Commands (unchanged)

```bash
agent-swarm init <project>   # Initialize swarm in a project (updated — see above)
agent-swarm watch          # Start watchdog (spawns agents)
agent-swarm status         # Show tracker state
agent-swarm tui            # Interactive TUI
agent-swarm done <id>      # Mark ticket done
agent-swarm fail <id>      # Mark ticket failed
agent-swarm add-ticket     # Add a ticket manually
agent-swarm integrate      # Run integration merge
agent-swarm cleanup        # Clean up worktrees
agent-swarm archive        # Archive done tickets
```

---

## 6. Watchdog Changes

### 6.1 Multi-Model Spawning

Current watchdog spawns all agents with the same model (`gpt-5.3-codex`). New behavior:

1. Read ticket type from tracker (derived from ID pattern or explicit `type` field)
2. Look up profile for that ticket type
3. Read model from profile frontmatter
4. Spawn with correct model:
   - Codex model → `codex exec -m gpt-5.3-codex ...` (tmux)
   - Sonnet model → **New backend**: Claude API call or Claude Code session
   - Opus model → **New backend**: Claude API call

**Implementation note:** Sonnet/Opus agents need a non-Codex backend. Options:
- (a) Use `claude` CLI if available
- (b) Direct API calls via Go HTTP client
- (c) Only use Codex for everything, inject profile as prompt (fallback)

**Decision needed at build time.** For v2.0, recommend (c) as fallback — all agents use Codex with profile-injected prompts. Add native sonnet/opus backend in v2.1.

### 6.2 Post-Build Auto-Generation

When watchdog detects all build tickets for a feature are `done`:
1. Auto-create the 7 post-build tickets in tracker
2. Auto-generate prompts for each (using profile + feature context)
3. Set deps: `int` first, then `gap`+`tst` parallel, then `review`+`sec` parallel, then `doc`+`clean` parallel
4. Advance feature state to `post_build`

### 6.3 Fix Ticket Auto-Creation

When watchdog detects a `review-*` or `sec-*` ticket completed:
1. Read the findings JSON from feature directory
2. Filter for `critical` and `high` severity
3. Create `fix-<feature>-<N>` tickets for each finding
4. Generate prompts from finding details
5. Add to current phase with dep on the review ticket
6. If no critical/high findings, advance normally

### 6.4 Completion Detection

Feature is auto-advanced to `complete` candidate when:
- All 7 post-build tickets are `done`
- All fix tickets (if any) are `done`
- No `BLOCK` verdicts remain unresolved

Navi notifies Mike for final sign-off.

---

## 7. Configuration Changes

### 7.1 swarm.toml Additions

```toml
[project]
name = "my-project"
repo = "~/projects/my-project"
base_branch = "dev"
max_agents = 5
auto_approve = false
model = "gpt-5.3-codex"
effort = "high"
bypass_sandbox = true
spec_file = "SPEC.md"                    # existing
features_dir = "swarm/features"           # NEW

[profiles]                                # NEW section — paths relative to project root
architect = ".agents/profiles/architect.md"
code_agent = ".agents/profiles/code-agent.md"
tdd_guide = ".agents/profiles/tdd-guide.md"
code_reviewer = ".agents/profiles/code-reviewer.md"
security_reviewer = ".agents/profiles/security-reviewer.md"
e2e_runner = ".agents/profiles/e2e-runner.md"
doc_updater = ".agents/profiles/doc-updater.md"
refactor_cleaner = ".agents/profiles/refactor-cleaner.md"
build_error_resolver = ".agents/profiles/build-error-resolver.md"

[post_build]                              # NEW section
# Dependency order for post-build tickets
# int → gap+tst (parallel) → review+sec (parallel) → doc+clean (parallel)
order = ["int", "gap", "tst", "review", "sec", "doc", "clean", "mem"]
parallel_groups = [["gap", "tst"], ["review", "sec"], ["doc", "clean"]]
# mem runs last — after all other post-build tickets, so it can capture everything

[notifications]
telegram_token_cmd = "pass show apis/telegram-bot-token"
telegram_chat_id = "1556514337"
```

### 7.2 Tracker Changes

Ticket schema gains optional fields:
```json
{
  "id": "review-cache",
  "type": "review",           // NEW: ticket type
  "feature": "cache-overhaul", // NEW: feature association
  "profile": "code-reviewer",  // NEW: agent profile to use
  "desc": "Code review for cache overhaul feature",
  "phase": 2,
  "status": "todo",
  "deps": ["int-cache"]
}
```

---

## 8. Legacy Governance Cleanup

### Files to Remove
- Legacy governance binaries no longer required by this workflow
- Legacy governance cache directories in projects
- Legacy-governance sections from `CODEX.md` files
- Legacy-governance sections from `swarm/prompt-footer.md`

### Files to Update
- `~/.openclaw/workspace/templates/AGENTS.md` → keep TDD + Karpathy + Git hygiene, remove legacy governance steps
- All project `AGENTS.md` files → regenerate from updated template
- `swarm/prompt-footer.md` → remove external-governance Phase 0 validation step

---

## 9. Migration Path

### Phase 1: Foundation (agent-swarm code changes)
1. Add `feature` subcommand with state machine
2. Add `type`, `feature`, `profile` fields to tracker schema
3. Add `[profiles]` and `[post_build]` config sections
4. Update prompt auto-generation to use profiles + feature context
5. Remove external-governance preflight from prompt-footer
6. Add `validate` command

### Phase 2: Post-Build Automation
7. Watchdog: detect all-build-done → auto-create post-build tickets
8. Watchdog: detect review-done → parse findings → auto-create fix tickets
9. Review/security prompt suffix for structured JSON output

### Phase 3: Multi-Model (v2.1, future)
10. Add Claude API backend for sonnet/opus agents
11. Model selection from profile frontmatter
12. Separate tmux sessions with different backends

### Phase 4: Cleanup
13. Remove legacy governance references from all projects
14. Update AGENTS.md template
15. Update workspace docs (AGENTS.md, SOUL.md references)
16. Archive/deprecate old legacy workflow artifacts and references

---

## 10. Backward Compatibility

- Existing `swarm.toml` files without `[profiles]`/`[post_build]` still work — defaults to current behavior (all tickets use project model, no post-build auto-creation)
- Feature commands are additive — projects can still use raw `add-ticket` + `watch` without features
- Tracker schema additions are optional fields — old trackers load fine
- `validate` command works on both old and new format

---

## 11. Ticket Estimate

| Phase | Tickets | Estimate |
|-------|---------|----------|
| 1: Foundation | 6 | ~18h |
| 2: Post-Build Automation | 3 | ~12h |
| 3: Multi-Model (v2.1) | 3 | ~10h |
| 4: Cleanup | 4 | ~4h |
| **Total** | **16** | **~44h** |

Phase 3 (multi-model) can be deferred — v2.0 runs everything on Codex with profile injection. Still valuable because the profile content shapes agent behavior even without model switching.

---

## 12. Success Criteria

After v2.0:
1. `agent-swarm feature add cache-overhaul --prd prd.md` creates the feature
2. `agent-swarm feature approve-prd cache-overhaul` gates on Mike
3. Architect agent runs automatically, outputs ADR
4. `agent-swarm feature approve-spec cache-overhaul` gates on Mike
5. `agent-swarm feature plan cache-overhaul` creates tickets + prompts
6. `agent-swarm watch` spawns build agents with TDD prompts
7. When build done → 7 post-build tickets auto-created and spawned
8. Review findings → critical/high auto-create fix tickets
9. All done → Mike notified for final sign-off
10. Every step is deterministic — no drift between sessions
