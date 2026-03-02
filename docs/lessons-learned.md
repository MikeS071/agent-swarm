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
- **Decapod/governance commands that fail** — agents try to fix pre-existing failures instead of ignoring them
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

## Decapod Integration

### Current state
- `decapod validate` runs in agent worktrees but reports ~25 pre-existing invariant failures
- Agents capture `validation_output.json` at start (baseline) and end (post-work)
- The validation artifact is committed with agent work for orchestrator review
- No new failures = agent passed governance gate
- Pre-existing failures need separate remediation (not agent responsibility)

### Footer pattern
```
### Phase 0: Decapod governance
Run once at start — capture validation state:
  export PATH="$HOME/.cargo/bin:$PATH"
  decapod validate --format json > validation_output.json 2>&1

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
