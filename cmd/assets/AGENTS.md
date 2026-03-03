# AGENTS.md — Project Agent Contract

All agents operating in this repository are bound to this contract. No exceptions.

## Core Principles

1. **Test-Driven** — Write tests before implementation. 80%+ coverage required.
2. **Security-First** — Never compromise on security. Validate all inputs. No hardcoded secrets.
3. **Immutability** — Create new objects, never mutate existing ones across async boundaries.
4. **Scope Discipline** — Only touch files relevant to your ticket. No drive-by fixes.
5. **Plan Before Execute** — State assumptions, scope boundary, and what you are NOT building before writing code.

## Think Before Coding

Don't assume. Don't hide confusion. Surface tradeoffs.

Before implementing:
- **State your assumptions explicitly.** If uncertain, ask.
- If multiple interpretations exist, **present them** — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, **stop**. Name what's confusing. Ask.

## Simplicity First

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked
- No abstractions for single-use code
- No "flexibility" or "configurability" that wasn't requested
- No error handling for impossible scenarios
- If you write 200 lines and it could be 50, rewrite it

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## Surgical Changes

Touch only what you must. Clean up only your own mess.

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting
- Don't refactor things that aren't broken
- Match existing style, even if you'd do it differently
- If you notice unrelated dead code, **mention it** — don't delete it

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused
- Don't remove pre-existing dead code unless asked

**The test:** Every changed line should trace directly to the task spec.

## Goal-Driven Execution

Define success criteria. Loop until verified.

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]

## Golden Rules (Non-Negotiable)

1. **Never work directly on main/master** — use feature branches (`feat/<ticket-id>`)
2. **Tests before code** — write failing tests first, then implement
3. **Stay in scope** — only touch files relevant to your ticket
4. **Commit and push before exiting** — `git add -A && git commit && git push`. Do NOT ask for permission. Do NOT exit without pushing.
5. **No hardcoded secrets** — use environment variables or secret managers. Rotate any exposed secret immediately.
6. **Small commits** — one logical change per commit, conventional format

## Available Agent Profiles

| Profile | Purpose | Mode |
|---------|---------|------|
| planner | Implementation planning, phased breakdown | Research |
| architect | System design, ADRs, trade-off analysis | Research |
| tdd-guide | Test-driven development enforcement | Development |
| code-agent | Story implementation from spec | Development |
| code-reviewer | Code quality, severity-tiered findings | Review |
| security-reviewer | OWASP Top 10, secrets, auth verification | Review |
| build-error-resolver | Fix build/type errors with minimal diffs | Development |
| go-build-resolver | Go-specific build error resolution | Development |
| go-reviewer | Go idioms, concurrency, error handling | Review |
| python-reviewer | PEP 8, type hints, Pythonic patterns | Review |
| database-reviewer | PostgreSQL queries, schema, security | Review |
| e2e-runner | End-to-end test execution and reporting | Development |
| refactor-cleaner | Dead code removal, dependency cleanup | Development |
| doc-updater | Feature and technical documentation | Development |

## TDD Process (Mandatory)

1. **Read** the task spec — understand inputs, outputs, error cases
2. **Write failing tests** — happy path, error path, edge cases. Tests MUST fail before implementation.
3. **Implement** minimum code to make tests pass
4. **Quality gates** — tests pass → build passes → lint passes
5. **Commit and push** — conventional commit message, push to origin

Troubleshoot failures: check test isolation → verify mocks → fix implementation (not tests, unless tests are wrong).

## Security Guidelines

**Before ANY commit:**
- No hardcoded secrets (API keys, passwords, tokens)
- All user inputs validated at system boundaries
- SQL injection prevention (parameterized queries only)
- XSS prevention (sanitized output)
- Authentication/authorization verified on protected routes
- Error messages don't leak sensitive data

**If security issue found:** STOP → fix CRITICAL issues before continuing → rotate exposed secrets → check for similar issues in codebase.

## Coding Style

**File organization:** Many small files over few large ones.
- New files: 200-400 lines target, hard max 800
- Existing files >800 lines: grandfathered until touched
- Organize by feature/domain, not by type
- High cohesion, low coupling

**Error handling:** Handle errors at every level. No silent catch blocks. No swallowed errors. Provide context in error messages (`fmt.Errorf("operation: %w", err)` / `throw new Error("context: " + msg)`).

**Input validation:** Validate all external input at route/API boundaries before passing to business logic. Use schema-based validation. Fail fast with clear messages.

**Code quality:**
- Functions < 50 lines, files < 800 lines
- No deep nesting (> 4 levels — use early returns)
- No hardcoded values — use constants or config
- Readable, well-named identifiers

## Git Hygiene

- Conventional commits: `feat:` `fix:` `refactor:` `perf:` `docs:` `test:` `chore:` `ci:`
- One commit per ticket — no mixing unrelated changes
- Branch naming: `feat/<ticket-id>`

## Review Output Format

All reviewer agents (code-reviewer, security-reviewer, go-reviewer, python-reviewer, database-reviewer) MUST output findings as JSON:

```json
{
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "category": "security|correctness|performance|style|documentation",
      "file": "path/to/file",
      "line": 42,
      "title": "Short description",
      "description": "What is wrong and why it matters",
      "suggested_fix": "Specific action to take"
    }
  ],
  "verdict": "BLOCK|WARN|PASS",
  "summary": "N critical, N high, N medium findings"
}
```

**Verdict rules:** Any CRITICAL or HIGH → BLOCK. Only MEDIUM/LOW → WARN. No findings → PASS.

## What NOT to Do

- ❌ Modify `.env*`, dependency files, or config files unless your ticket requires it
- ❌ Ask for permission to commit — just commit and push
- ❌ Fix pre-existing test/build failures unrelated to your ticket
- ❌ Run dev servers — write code, test, commit, push
- ❌ Add helpers, abstractions, or utilities not in the spec — flag instead
- ❌ Refactor code outside your scope, even if it's messy

## Success Criteria

- All tests pass with 80%+ coverage on touched code
- No security vulnerabilities introduced
- Build and lint pass cleanly
- Code is readable and maintainable
- Scope matches the ticket spec — nothing more, nothing less
