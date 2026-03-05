# Ticket Prep + Post-Build Automation (TP)

**Added:** 2026-03-05

## Overview
TP adds strict preflight checks before orchestration and automatic post-build ticket creation after a feature's build tickets are complete. It reduces failed spawns by enforcing prompt/role/verify readiness and keeps post-build workflows consistent.

## How to Use
1. Define tickets with explicit `profile` and `verify_cmd` when required.
2. Create prompt files in `swarm/prompts/<ticket-id>.md`.
3. Run readiness checks:
   - `swarm doctor --json`
   - `swarm prep --json`
4. Start orchestration:
   - `swarm watch --once` for a control pass
   - `swarm watch` for continuous runs
5. When all build tickets for a feature are done and `post_build.order` is configured, the watchdog auto-creates feature post-build tickets (for example `int-<feature>`, `review-<feature>`, `doc-<feature>`).
6. For `review-<feature>` and `sec-<feature>` tickets, write structured JSON findings reports under `swarm/features/<feature>/`.

## Configuration
| Option | Default | Description |
|---|---|---|
| `project.require_explicit_role` | `true` | Blocks unprepared tickets that do not set `profile`. |
| `project.require_verify_cmd` | `true` | Requires ticket `verify_cmd`, unless integration fallback is set. |
| `integration.verify_cmd` | `""` | Fallback verify command used when a ticket omits `verify_cmd`. |
| `post_build.order` | `int,gap,tst,review,sec,doc,clean,mem` | Ordered post-build steps auto-generated per feature after build completion. |
| `post_build.parallel_groups` | `[[gap,tst],[review,sec],[doc,clean]]` | Adjacent steps that may run in parallel stages. |
| `guardian.enabled` | `true` | Enables strict guardian policy checks at runtime. |

## Limits & Quotas
- `swarm watch` is blocked when prep checks fail.
- Ticket spawn count is bounded by `project.max_agents` and `project.min_ram_mb`.
- Verify commands run in each ticket worktree; failures block done state.
- Auto-generated fix tickets from review/security reports are created only for `critical` and `high` findings.

## FAQ
**Q: What fails prep most often?**  
A: Missing prompt files, missing explicit `profile`, or missing `verify_cmd` with no `integration.verify_cmd` fallback.

**Q: Do review/security tickets write code changes?**  
A: No. Their prompt contract is read-only and expects JSON findings output under `swarm/features/<feature>/`.

**Q: What verify command do auto-generated post-build tickets use?**  
A: They inherit `integration.verify_cmd` when available.

**Q: Can post-build auto-creation be disabled?**  
A: Yes. If `post_build.order` is empty/unset, no post-build tickets are auto-generated.
