
---

## MANDATORY DEVELOPMENT PROCESS (appended automatically — follow in exact order)

### Phase 0: Decapod governance (run before any code)
This repo uses Decapod for governance. Run these commands first:
```bash
decapod validate                    # verify .decapod/ is healthy
decapod session acquire             # acquire a session for this work
decapod rpc --op agent.init         # register this agent run
decapod rpc --op context.resolve    # resolve scoped context for this task
```
Read `CODEX.md` and `AGENTS.md` in the project root for the full operational contract.
After completing all work, run `decapod rpc --op proof.validate` to generate proof artifacts.

### Phase 1: Understand the spec
- Read the task objective and requirements above
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
1. **Tests pass:** `go test ./... -count=1` (Go) or `pnpm test` (Web/TS)
2. **Build passes:** `go build ./...` (Go) or `pnpm build` (Web/TS)
3. **Types/Lint:** `go vet ./...` (Go) or `pnpm typecheck` (Web/TS)
4. Iterate until all green

### Phase 5: Commit (only after ALL gates pass)
```bash
git add -A
git commit -m "feat: <description>

Tests: X passed, 0 failed"
git push origin HEAD
```

Do NOT exit without committing and pushing.
Do NOT commit if any gate is failing.
Do NOT write implementation before tests.
