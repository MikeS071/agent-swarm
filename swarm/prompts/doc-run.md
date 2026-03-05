# doc-run

## Objective
Documentation update for Guardian G3-G5 completion state.

## Dependencies
g3-01, g3-02, g3-03, g4-01, g4-02, g4-03, g5-01, g5-02, g5-03, g5-04, int-g3, int-g4, int-g5, tst-g3, tst-g4, tst-g5

## Scope
- Update user-facing docs (`docs/user-guide.md`) where behavior changed.
- Update technical docs (`docs/technical.md`) with architecture/flow changes.
- Update release notes (`docs/release-notes.md`) for shipped impact.
- If no doc updates are required, provide explicit no-op justification.

## Inputs
- swarm/features/run/review-report.json
- swarm/features/run/sec-report.json
- swarm/features/run/gap-report.md

## Required deliverable
Always produce and commit:
- swarm/features/run/doc-report.md

## Verify
go build ./...
