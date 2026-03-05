# TP Documentation Report

## Inputs
- Expected but not found in this workspace:
  - `swarm/features/tp/review-report.json`
  - `swarm/features/tp/sec-report.json`
- Documentation was generated from implemented source code and tests.

## Docs Updated
- `docs/features/tp.md`
- `docs/technical/tp.md`
- `swarm/features/tp/doc-report.md`

## Key Behavior Documented
- Preflight hard gates (`swarm prep`, `swarm doctor`) for role/profile, verify command, and prompt presence.
- `swarm watch` preflight enforcement (watch blocked when prep fails).
- Post-build auto-ticket generation driven by `post_build.order` and `post_build.parallel_groups`.
- Integration verify command fallback behavior for ticket verification and generated post-build tickets.
- Review/security JSON output contract and automatic creation of fix tickets for critical/high findings.

## Deferred Docs Tasks
- Add a follow-up addendum if/when `swarm/features/tp/review-report.json` and `swarm/features/tp/sec-report.json` become available, to document any findings-specific operational guidance.
- Optionally cross-link TP docs from `docs/user-guide.md` and `docs/technical.md` if a centralized index is desired.
