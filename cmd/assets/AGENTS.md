# AGENTS.md — Project Agent Contract

All agents operating in this repository are bound to this contract.

## Golden Rules (Non-Negotiable)

1. **Never work directly on main/master** — use feature branches (`feat/<ticket-id>`)
2. **Tests before code** — write failing tests first, then implement
3. **Stay in scope** — only touch files relevant to your ticket. No drive-by refactors.
4. **Commit and push before exiting** — never exit without `git add -A && git commit && git push`
5. **No hardcoded secrets** — use environment variables or secret managers
6. **Small commits** — one logical change per commit, conventional format (`feat:`, `fix:`, `refactor:`, etc.)

## TDD Process (Mandatory)

1. **Read** the task spec and understand inputs, outputs, error cases
2. **Write failing tests** — happy path, error path, edge cases
3. **Implement** minimum code to make tests pass
4. **Quality gates** — tests pass, build passes, lint passes
5. **Commit and push** — conventional commit message, push to origin

## File Size Limits

- New files: 200-400 lines target, never exceed 800
- Existing files >800 lines: grandfathered until touched

## Git Hygiene

- Conventional commits: `feat:` `fix:` `refactor:` `perf:` `docs:` `test:` `chore:` `ci:`
- One commit per ticket — no mixing unrelated changes
- Branch naming: `feat/<ticket-id>`

## Review Output Format

If you are a reviewer (code-reviewer, security-reviewer), output findings as JSON:
```json
{
  "findings": [{"severity": "critical|high|medium|low", "category": "...", "file": "...", "line": 0, "title": "...", "description": "...", "suggested_fix": "..."}],
  "verdict": "BLOCK|WARN|PASS",
  "summary": "..."
}
```

## What NOT to Do

- Do NOT modify `.env*`, `package.json` deps, or config files unless your ticket requires it
- Do NOT ask for permission to commit — just commit
- Do NOT attempt to fix pre-existing test/build failures unrelated to your ticket
- Do NOT run dev servers — your job is to write code, test, commit, push
