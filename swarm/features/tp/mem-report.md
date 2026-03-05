# TP Memory Report

**Feature:** tp  
**Date:** 2026-03-05

## Input Status
Expected upstream inputs were not present in this branch:
- `swarm/features/tp/review-report.json`
- `swarm/features/tp/sec-report.json`
- `swarm/features/tp/doc-report.md`
- `swarm/features/tp/clean-report.md`

This report is consolidated from implemented code paths and existing project docs/tests.

## Key Decisions Taken (and Why)
1. Enforce explicit ticket role/profile and verify command by default.
- Why: reduce ambiguous agent behavior and enforce per-ticket accountability before spawn and before mark-done.
- Evidence: `internal/config/config.go` defaults `require_explicit_role=true`, `require_verify_cmd=true`; enforced in `cmd/prep.go`, `cmd/watch_runtime.go`, `internal/watchdog/watchdog.go`, and `internal/guardian/evaluator.go`.

2. Gate swarm execution with a strict preflight (`prep`).
- Why: fail fast on missing prompt files, missing role/profile, and missing verify command instead of failing deep in watchdog runtime.
- Evidence: `cmd/prep.go` and watch gate in `cmd/watch_runtime.go`.

3. Assemble prompts in layered order: governance -> spec -> profile -> ticket -> footer (+ review/security suffix).
- Why: preserve global constraints while still allowing ticket-specific work and deterministic reviewer outputs.
- Evidence: `internal/watchdog/watchdog.go` (`assemblePromptForTicket`, `reviewOutputSuffix`).

4. Route backend/model by profile frontmatter, with safe fallback.
- Why: allow role-specific model/backend tuning while preventing incompatible model/backend combinations from breaking spawn.
- Evidence: `internal/watchdog/watchdog.go` (`resolveSpawnModelForBackend`, `resolveSpawnBackendType`, compatibility fallback logging).

5. Auto-generate post-build tickets per feature using configured order and safe parallel groups.
- Why: standardize integration/review/security/docs/cleanup/memory sequencing after build completion with deterministic dependencies.
- Evidence: `internal/watchdog/watchdog.go` (`ensurePostBuildTickets`, `buildPostBuildStages`, `createPostBuildTicketsForFeature`, `postBuildProfile`, `postBuildDescription`).

6. Keep backward compatibility for tracker location by one-time fallback import.
- Why: support migration to `state_dir` without breaking existing projects that still have repo-local `swarm/tracker.json`.
- Evidence: `cmd/state_tracker.go`.

## Important Choices and Trade-offs
1. Strict defaults vs onboarding friction.
- Choice: strict fields enabled by default.
- Trade-off: higher initial setup burden, but fewer runtime failures and less undefined behavior.

2. Naming-convention inference vs explicit metadata.
- Choice: infer build and post-build feature IDs from ticket naming when type/feature is missing.
- Trade-off: supports older data, but correctness depends on stable ticket naming patterns.

3. Auto-generated prompts vs preserving manual edits.
- Choice: generate post-build prompt files only when absent.
- Trade-off: safer for manual customization, but existing stale prompts are not auto-refreshed.

4. Structured reviewer outputs vs autonomous fixing.
- Choice: reviewer/security roles are read-only and must emit JSON reports.
- Trade-off: cleaner automation inputs, but remediation requires follow-on tickets.

## Lessons Learned and Recommended Defaults
1. Keep `project.require_explicit_role = true` and `project.require_verify_cmd = true` for all production projects.
2. Set a non-empty `integration.verify_cmd` to avoid per-ticket verify command drift.
3. Prefer explicit `type` and `feature` fields on tracker tickets; rely on ID parsing only for legacy compatibility.
4. Keep `post_build.order` aligned with delivery flow (`int, gap, tst, review, sec, doc, clean, mem`) unless a project has a measured reason to deviate.
5. Use non-overlapping, contiguous `post_build.parallel_groups`; invalid groups are intentionally ignored by stage builder.
6. Treat preflight (`agent-swarm prep`) as mandatory before starting watchdog.
7. Close current test gap: add unit tests for `runPrepChecks` (happy path, missing role/verify, missing prompt).

## Source Evidence (Primary)
- `internal/config/config.go`
- `cmd/prep.go`
- `cmd/watch_runtime.go`
- `internal/watchdog/watchdog.go`
- `internal/guardian/evaluator.go`
- `cmd/state_tracker.go`
- `internal/watchdog/watchdog_test.go`
- `internal/watchdog/routing_test.go`
- `cmd/feature_test.go`
