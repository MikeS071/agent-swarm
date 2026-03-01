# AGENTS.md — Universal Agent Contract

This is a Decapod-managed repository. All agents (Claude, Codex, Gemini, or any other) operating here are bound to this contract.

## Decapod Governance

### Mandatory Initialization
```bash
decapod validate                    # verify .decapod/ is healthy
decapod docs ingest                 # ingest constitution docs
decapod session acquire             # acquire session for this work
decapod rpc --op agent.init         # register this agent run
decapod workspace status            # check workspace state
decapod rpc --op context.resolve    # resolve scoped context for task
```

### Control-Plane Loop
```bash
decapod capabilities --format json
decapod data schema --deterministic
decapod docs search --query "<problem>" --op <op>
decapod rpc --op context.scope --params '{"query":"<problem>","limit":8}'
```

### Proof & Completion
```bash
decapod workunit init --task-id <task-id> --intent-ref <intent>
decapod rpc --op proof.validate     # generate proof artifacts on completion
decapod validate                    # must pass before claiming done
```

### Cross-Agent Coordination
- Use `decapod todo handoff --id <id> --to <agent>` for ownership transfer
- Use `decapod data memory add|get` for shared preferences across sessions/agents
- Treat lock/contention failures (`VALIDATE_TIMEOUT_OR_LOCK`) as blocking until resolved

## Golden Rules (Non-Negotiable)

1. Always refine intent with the user before inference-heavy work
2. Never work directly on main/master — use feature branches or assigned worktrees
3. `.decapod` files are accessed only via the decapod CLI
4. Never claim done without `decapod validate` passing
5. Never invent capabilities not exposed by the binary
6. Stop if requirements conflict, intent is ambiguous, or policy boundaries are unclear
7. Respect the Interface abstraction boundary

## Engineering Principles

- Ship baseline first, improve after. Correctness > cleverness.
- Small isolated reversible changes. Measure with concrete outputs.
- One hypothesis at a time. Optimise bottlenecks only.
- Validate full end-to-end path. Be honest about speculative fixes.

## Karpathy Coding Standards

### Before coding
- State assumptions and ambiguity up front. No silent guesses.
- Define success criteria before writing a single line.
- Smallest solution that meets requirements — explicitly state what you are NOT building.

### During coding
- Surgical changes only: touch only scope-required files.
- Immutability by default, especially across async/concurrent boundaries.
- Small cohesive files/modules: target 200–400 lines, never exceed 800.
- Explicit error handling everywhere — no silent catch blocks, no swallowed errors.
- Validate all external input at route/API boundaries before passing to business logic.
- No hardcoded secrets — all config via environment variables or secret store.
- Use parallel analysis/execution for independent tasks.

### Progress discipline
- No moving to next feature while any test is failing.
- After each feature or work gap, checkpoint: what was achieved, % complete, total LOC in scope.
- Test rigor is a gate, not a suggestion.

## Mandatory Development Process

### Phase 1: Understand the spec
- Read the task objective and requirements
- Identify every behaviour, input, output, and error case

### Phase 2: Write tests FIRST (before any implementation code)
- Write failing tests that define the expected behaviour from the spec
- Min 3 test cases per function: happy path, error path, edge case
- Table-driven tests where applicable
- Mock external dependencies (DB, HTTP, Docker) — no real connections
- Run tests — they SHOULD fail (red). This confirms they test real behaviour.

### Phase 3: Implement
- Write the minimum code to make all tests pass
- Do NOT write code that isn't covered by a test

### Phase 4: Quality gates (run in order, fix and re-run from gate 1 on failure)
1. **Tests pass:** `go test ./... -count=1` (Go) or `pnpm test` (TS)
2. **Build passes:** `go build ./...` (Go) or `pnpm build` (TS)
3. **Types/Lint:** `go vet ./...` (Go) or `pnpm typecheck` (TS)
4. **Decapod validate:** `decapod validate`
5. Iterate until all green

### Phase 5: Commit (only after ALL gates pass)
```bash
git add -A
git commit -m "<type>: <description>

Tests: X passed, 0 failed
Decapod: validated"
git push origin HEAD
```

Do NOT exit without committing and pushing.
Do NOT commit if any gate is failing.
Do NOT write implementation before tests.

## Git Hygiene

- Conventional commits: `feat:` `fix:` `refactor:` `perf:` `docs:` `test:` `chore:` `ci:`
- One commit per story — no mixing unrelated cleanup with behaviour changes
- Include verification evidence (test count, build status) in commit body for significant changes
- Never push directly to main/master — use feature branches

## Safety

- No private data exfiltration. Ever.
- Prefer non-destructive operations (`trash` > `rm`)
- No hardcoded secrets, API keys, or tokens in source
- All config via environment variables or `pass` store
- Ask before destructive operations

## Agent Modes (declare at task start)

- **Development Mode** — implement first, explain after; small atomic diffs; validate every change with runnable checks
- **Research Mode** — gather evidence before editing; confirm assumptions with code/doc inspection; findings first, recommendations second
- **Review Mode** — severity order: `critical > high > medium > low`; focus on correctness, security, performance regressions, test gaps

## Safety Invariants

- ✅ Router pointer: `core/DECAPOD.md`
- ✅ Validation gate: `decapod validate`
- ✅ Constitution ingestion gate: `decapod docs ingest`
- ✅ Workspace status gate: `decapod workspace status`
- ✅ Session auth gate: `DECAPOD_SESSION_PASSWORD`
- ✅ Privilege gate: request elevated permissions before Docker/container workspace commands
