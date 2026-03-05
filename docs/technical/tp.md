# Ticket Prep + Post-Build Automation (TP) — Technical Reference

**Added:** 2026-03-05

## Architecture
TP behavior is implemented as runtime gates and watchdog orchestration logic:
- `prep`/`doctor` run shared readiness checks (`profile`, `verify_cmd`, prompt existence).
- `watch` enforces the prep gate before any execution.
- `watchdog.RunOnce` auto-creates post-build tickets per feature when build tickets are complete and `post_build.order` is configured.
- Review/security prompt assembly appends a strict JSON output contract.
- Completing `review-*` or `sec-*` tickets can auto-create `fix-<feature>-N` tickets from high/critical findings.

## Key Files
| File | Purpose |
|---|---|
| `cmd/prep.go` | Defines `runPrepChecks` and `swarm prep`. |
| `cmd/doctor.go` | Exposes gate status and prep issues via `swarm doctor`. |
| `cmd/watch_runtime.go` | Blocks watch execution when prep checks fail. |
| `internal/watchdog/watchdog.go` | Prompt assembly, verify fallback, post-build generation, fix-ticket generation. |
| `internal/watchdog/watchdog_test.go` | Coverage for post-build generation, idempotency, review/sec fix generation, suffix contracts. |
| `internal/config/config.go` | Default `post_build` order/groups and gate defaults. |
| `internal/dispatcher/dispatcher.go` | Spawn gating with role/verify checks plus phase constraints. |

## Data Flow
1. `swarm prep`/`swarm doctor` loads config + tracker and validates each non-done ticket.
2. `swarm watch` calls the same preflight checks and exits early on failure.
3. `watchdog.RunOnce` reconciles running tickets, verifies completion, and updates tracker/event log.
4. If a feature's build tickets are fully done and post-build is enabled, watchdog adds missing post-build tickets and prompt files.
5. If `review-*`/`sec-*` finishes and a report exists at `swarm/features/<feature>/review-report.json` or `sec-report.json`, watchdog parses findings and creates dependent `fix-*` tickets for actionable severity.

## API Surface
| Method | Path | Auth | Description |
|---|---|---|---|
| N/A | N/A | N/A | TP changes in this repo are CLI/runtime behavior; no TP-specific HTTP route was added. |

## Schema Changes
Tracker ticket JSON supports TP-related fields used by runtime behavior:
- `type` (post-build step classification, e.g., `int`, `doc`, `sec`)
- `feature` (feature key bound to generated post-build and fix tickets)
- `run_id` (optional run context)
- `profile` and `verify_cmd` are enforced by prep/dispatch gates when required.

No database migration is required; state is stored in tracker JSON.

## Known Edge Cases
- Missing review/security report file after ticket completion: watchdog logs warning and continues without fix-ticket creation.
- Invalid report JSON: non-fatal; no fix tickets are generated from that report.
- Empty `post_build.order`: post-build autogeneration is disabled.
- Missing ticket `verify_cmd`: runtime falls back to `integration.verify_cmd`; if both absent and `require_verify_cmd=true`, completion is blocked.
- Non-contiguous/overlapping `post_build.parallel_groups`: ignored during stage construction.

## Tests
Primary TP behavior coverage exists in:
- `internal/watchdog/watchdog_test.go`:
  - post-build auto-creation, stage dependencies, idempotency, dry-run non-mutation
  - review/sec suffix contract in assembled prompts
  - fix-ticket generation from high/critical findings only
  - verify command inheritance expectations for generated post-build tickets
