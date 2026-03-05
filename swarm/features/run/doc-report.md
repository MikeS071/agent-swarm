# Documentation Report — run

**Date:** 2026-03-05

## Inputs Reviewed

Expected inputs were not present in this worktree:
- `swarm/features/run/review-report.json` (missing)
- `swarm/features/run/sec-report.json` (missing)
- `swarm/features/run/gap-report.md` (missing)

Documentation updates were generated from current source code and tests in `cmd/`, `internal/server/`, and `internal/watchdog/`.

## Docs Changed and Why

| Document | Change | Why |
|---|---|---|
| `docs/user-guide.md` | Expanded API server section with watchdog run trigger (`POST /api/watchdog/run`), auth-token behavior, and endpoint list refresh. | User-facing run behavior needed clear invocation and auth guidance. |
| `docs/technical.md` | Added run execution paths (`watch`, `watch --once`, API run endpoint), server middleware/auth behavior, and watchdog run endpoint status semantics. | Technical docs needed accurate request/flow details from implementation. |
| `docs/release-notes.md` | Added 2026-03-05 entry for run-control documentation and API behavior clarification. | Release notes needed shipped impact for run behavior visibility. |

## Scope and Intent Coverage Check

- User-facing behavior changes documented in `docs/user-guide.md`: `Yes`
- Technical architecture/flow changes documented in `docs/technical.md`: `Yes`
- Release impact captured in `docs/release-notes.md`: `Yes`
- Required artifact `swarm/features/run/doc-report.md` created: `Yes`

## Deferred Follow-Ups

- Provide the missing `review-run`/`sec-run`/`gap` artifacts if doc wording must reflect specific audit findings; no finding-specific language was added without source artifacts.
