# CLAUDE.md — Claude Code Agent Entrypoint

You are working in a Decapod-managed repository.
See `AGENTS.md` for the full universal contract.

## Quick Start

```bash
decapod validate
decapod docs ingest
decapod session acquire
decapod rpc --op agent.init
decapod workspace status
decapod rpc --op context.resolve
```

## Operating Mode

- Work in your assigned worktree or project directory
- Call `decapod workspace status` at startup and before implementation work
- `.decapod` files are accessed only via the decapod CLI
- Read canonical router: `decapod docs show core/DECAPOD.md`
- Capability authority: `decapod capabilities --format json`
- Scoped context: `decapod docs search --query "<problem>" --op <op>` or `decapod rpc --op context.scope`
- Shared memory: `decapod data memory add|get` for cross-session preferences

## Development Process (mandatory — see AGENTS.md for full details)

1. **Understand** the spec — identify behaviours, inputs, outputs, error cases
2. **Tests first** — write failing tests before any implementation
3. **Implement** — minimum code to pass all tests
4. **Quality gates** — tests → build → lint → `decapod validate` → all must pass
5. **Commit** — conventional commit, push, include test evidence

Stop if requirements are ambiguous or conflicting.
