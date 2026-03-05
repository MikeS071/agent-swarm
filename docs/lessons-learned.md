# Agent Swarm — Lessons Learned

Hard-won operational knowledge from running 60+ agent tickets across multiple projects.

## Prompt Engineering

### What works
- **Minimal, focused prompts** with exact file paths and test commands
- **Reference an existing file** for the pattern: "Look at `src/__tests__/api/activity.test.ts` for the mocking pattern"
- **Explicit test command**: "Run: `npx jest src/__tests__/api/foo.test.ts --ci`"
- **"Commit when done"** as the last line — simple, clear exit criterion

### What kills agents
- **Bloated prompt footers** with multi-phase ceremony — agents spend all their time on process instead of output
- **Governance commands that fail** — agents try to fix pre-existing failures instead of ignoring them
- **Vague scope** — "write tests for the admin module" vs "write tests for `src/app/api/admin/provision/route.ts`"
- **Broad review instructions** — "review the codebase for issues" causes destructive overreach

### Prompt footer rules
1. Keep it under 40 lines
2. Governance validation = capture output, distinguish pre-existing vs new failures, move on
3. TDD process should be: understand spec → read source → write tests → run → fix → commit
4. Always include: "Do NOT exit without committing and pushing"
5. Always include pattern reference to an existing test file
6. Add `.codex-prompt.md` to `.gitignore` — agents get confused by their own prompt file at commit time

### Prompt template (proven effective)
```markdown
# <ticket-id>: <title>

<1-3 sentences of context>

File(s) to test/implement: <exact paths>
Output file: <exact path>

Reference <existing-similar-file> for the pattern.

<3-6 bullet points of specific requirements>

Run: <exact test/build command>
All tests must pass. Commit when done.

---
<prompt footer>
```

## Watchdog Operations

### Failure modes encountered
1. **Watchdog crashes on missing worktree** — `HasCommits()` tried `chdir` into deleted dir
   - Fix: Check `os.Stat()` before git operations, return false gracefully
2. **Failed tickets block phase gate** — `phaseGateReached()` required ALL done
   - Fix: Phase gate still requires all done (failed = incomplete), but dispatcher now spawns across phases for any ticket whose deps are satisfied
3. **Stale branches block worktree creation** — `git worktree add -b feat/X` fails if branch exists
   - Fix: Clean branches + worktrees before resetting tickets. Future: agent-swarm should handle this automatically
4. **Tracker shows "running" for dead agents** — tmux session exits but watchdog hasn't checked yet
   - Fix: Watchdog resilience — log and continue, don't crash on any single ticket error
5. **`runningCount == 0` gate** — watchdog only spawned when ALL agents done, leaving slots idle
   - Fix: Spawn when `runningCount < maxAgents`
6. **Binary "Text file busy"** — can't overwrite binary while watchdog is using it
   - Fix: Kill watchdog, wait 2s, copy binary, restart

### Operational rules
- Always `git worktree prune` + delete stale branches before restarting a watchdog
- After resetting tickets to `todo`, verify no stale `feat/<id>` branches or `.-worktrees/<id>/` dirs exist
- Watchdog log (`swarm/watchdog-v2.log`) is the primary diagnostic — check it first
- The `idle check: signal=X, spawnable=N, running=N` log line tells you exactly why the watchdog isn't spawning

## Agent Behaviour Patterns

### Common agent failure modes
1. **Exits without committing** — most common. Agent finishes work but doesn't `git add && git commit && git push`
   - Mitigation: "Do NOT exit without committing and pushing" in footer
   - Recovery: Check worktree for uncommitted files, copy to dev manually
2. **Gets confused by `.codex-prompt.md`** — asks permission to commit an "unexpected file"
   - Fix: Add `.codex-prompt.md` to `.gitignore`
3. **Tries to fix pre-existing failures** — governance validation, type errors, lint warnings
   - Fix: Explicit "these are pre-existing, not your problem" in prompt
4. **Imports non-existent exports** — writes tests assuming functions exist
   - Prevention: Prompt should specify exactly which exports the route has
5. **Spends all time reading codebase** — never gets to writing code
   - Prevention: Point to ONE reference file, not "explore the codebase"

### Recovery workflow
When an agent fails but wrote useful code:
1. Check `.-worktrees/<id>/` for uncommitted test files
2. `git -C .-worktrees/<id> status --short` — look for `??` (untracked) and `M` (modified)
3. Copy the files to dev: `cp .-worktrees/<id>/src/__tests__/api/foo.test.ts src/__tests__/api/`
4. Run tests: `npx jest src/__tests__/api/foo.test.ts --ci`
5. Fix if needed, commit to dev
6. Mark ticket done in tracker

## Scaling

### Proven configurations
- **5 agents** on 8GB RAM server — safe, responsive
- **7 agents** — works but monitor with `free -m`
- **30s watchdog interval** — fast chaining, acceptable overhead
- **60m max_runtime** — generous for test-writing; reduce for simpler tasks

### When to use auto_approve
- Test backfill (no interdependencies between phases)
- Trusted pipelines where phase review adds no value
- NOT for feature work where phase N builds on phase N-1 output

## Governance Validation Integration

### Current state
- Validation commands often run in agent worktrees but report pre-existing invariant failures
- Agents capture `validation_output.json` at start (baseline) and end (post-work)
- The validation artifact is committed with agent work for orchestrator review
- No new failures = agent passed governance gate
- Pre-existing failures need separate remediation (not agent responsibility)

### Footer pattern
```
### Phase 0: Governance validation
Run once at start — capture validation state:
  <governance-validate-command> > validation_output.json 2>&1

Review validation_output.json. If there are failures:
- Distinguish PRE-EXISTING failures from NEW failures
- Do NOT attempt to fix pre-existing failures
- If your changes introduce NEW failures, fix them before proceeding
- Commit validation_output.json with your final commit
```

## Bugs Fixed (2026-03-02, commits 6ed4824 → 20c21bd)

### Dispatcher bugs
1. **Stale branch blocks spawn forever** — worktree `Create()` failed on existing branch, ticket stayed `todo`, retried every 30s infinitely
   - Fix: Delete existing branch before `git worktree add -b`
2. **`spawnableAcrossPhases()` ignored phase gates** — with `auto_approve=false`, tickets from Phase 5 could spawn while Phase 1 was still running
   - Fix: Cap spawnable tickets to `≤ currentPhase` when strict gates are on
3. **Idle spawn only ran when 0 agents running** — `runningCount < maxAgents` gate prevented filling available slots when some agents were already active
   - Fix: Always evaluate capacity; `CanSpawnMore()` inside loop handles the real check

### Watchdog bugs
4. **No spawn failure tracking** — failed spawns retried forever with no backoff or failure marking
   - Fix: `spawnErrors` map tracks consecutive failures; 3 failures → ticket marked failed + notification
5. **TUI Phase column always showed P0** — `Phase` field on `ticketRow` struct was never populated in `rebuildRows()`
   - Fix: Set `Phase: tk.Phase` when building rows

## Standard Phase Flow (mandatory unless auto_approve overrides)

Every phase follows this sequence — no exceptions:

1. **Feature tickets** run in parallel (respecting deps within phase)
2. **`int-N`** (integration) — merges all phase branches to dev, resolves conflicts, type-check + tests pass
3. **`tst-N`** (test suite) — full E2E test + build verification + coverage
4. **Phase gate** — watchdog stops, notifies human
5. **Human verifies** — checks deployed dev site, tests functionality
6. **Fix cycle** — if issues found, fix before approving
7. **Human approves** — `swarm go` or TUI `g` key → next phase starts

This is the default. `auto_approve = true` skips steps 4-7 (used for trusted pipelines like test backfill).

### Ticket naming convention
- Feature: `mc-01` through `mc-NN`
- Integration: `int-N` (one per phase, depends on all phase feature tickets)
- Test: `tst-N` (one per phase, depends on `int-N`)

### Integration ticket responsibilities
- Merge all phase feature branches to dev
- Resolve merge conflicts
- `npx tsc --noEmit` — zero type errors
- `npx jest --ci` — all tests pass
- Push to dev (triggers auto-deploy via Coolify)

### Test ticket responsibilities
- `npx tsc --noEmit` — zero type errors
- `npx jest --ci --coverage` — all tests pass + coverage report
- `npm run build` — production build succeeds
- Fix any failures before committing

## Codex exit code 1 after successful work

Codex CLI frequently returns exit code 1 even when it completes successfully and commits. This happens when:
- Any subprocess returned non-zero during the session (even if fixed afterwards)
- The summary report lists any "issues" encountered
- Internal error counter > 0

**This is expected and not a bug.** The watchdog correctly handles this by checking for commits (not exit codes). If commits exist on the branch → ticket is done. If no commits → retry/fail.

Never use exit codes to determine agent success. Always check git state.

### Bug: Shift+A triggered archive instead of approve (2026-03-02)
**Root cause:** `strings.ToLower(msg.String())` converted `A` to `a`, matching the archive case before the approve case.
**Fix:** Use raw `msg.String()` for case-sensitive keys. Removed `a` as archive trigger — archive is CLI-only now.
**Lesson:** Never use `ToLower()` on key input when you have case-sensitive keybindings.

### Bug: TUI phase approval didn't trigger watchdog spawning (2026-03-02)
**Root cause:** TUI wrote `unlocked_phase` to tracker.json, but the watchdog's in-memory Dispatcher kept the stale value. No disk reload between passes.
**Fix:** Watchdog re-reads `unlocked_phase` from tracker file at start of each `RunOnce()`. If disk value is higher, syncs via `SetUnlockedPhase()`.
**Lesson:** When multiple processes share state via a file, each must reload on every pass.

### Bug: auto_approve never worked from TOML (2026-03-02)
**Root cause:** `auto_approve` in TOML lives under `[project]`, but Go struct had it on `WatchdogConfig`. TOML parser mapped it to nothing.
**Fix:** Moved `AutoApprove` field from `WatchdogConfig` to `ProjectConfig`.
**Lesson:** TOML section names must match Go struct nesting exactly.

### Bug: TUI auto/manual toggle ignored by watchdog (2026-03-02)
**Root cause:** Watchdog loaded config once at startup and never re-read it. TUI wrote `auto_approve` to `swarm.toml` but the in-memory config stayed stale.
**Fix:** Watchdog re-reads `auto_approve` from `swarm.toml` on every `RunOnce()` pass. Dispatcher auto-advances phase gates when auto is true.
**Lesson:** Any setting togglable at runtime must be re-read from disk by all consumers on every pass.

## Ticket Prep V2 (TP) Defaults (2026-03-05)

### Decisions to keep
- Keep strict ticket preflight fields on by default: `require_explicit_role=true`, `require_verify_cmd=true`.
- Keep layered prompt composition order fixed: `AGENTS.md -> spec -> profile -> ticket -> footer`.
- Keep reviewer/security roles read-only with structured JSON outputs under `swarm/features/<feature>/`.
- Keep automatic post-build ticket generation enabled with explicit order and dependency staging.

### Trade-offs to remember
- Strict preflight catches failures early but increases initial ticket authoring work.
- ID-based feature inference (`<feature>-<n>`, `<step>-<feature>`) preserves legacy compatibility but is less robust than explicit `type` + `feature` fields.
- Post-build prompts are only created when missing, which protects manual edits but can leave stale templates in place.

### Recommended operational defaults
- Always set a project-wide `integration.verify_cmd` so post-build tickets inherit one consistent verification command.
- Require each ticket to declare a `profile` explicitly instead of relying on inferred defaults.
- Prefer explicit tracker metadata (`type`, `feature`) for all newly created tickets.
- Run `agent-swarm prep --config <path>` before every watchdog session.
- When customizing `post_build.parallel_groups`, keep groups contiguous and non-overlapping in `post_build.order`.

### Follow-up quality gap
- Add direct unit tests for `runPrepChecks` in `cmd/prep.go` (happy path, missing fields, missing prompt file) to lock strict-preflight behavior.
