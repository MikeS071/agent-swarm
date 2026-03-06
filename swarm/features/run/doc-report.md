# doc-run report

## Docs changed and why

1. `docs/user-guide.md`
   - Updated init output to include state tracker seed + default `swarm/flow.v2.yaml` scaffold.
   - Added `[guardian]` config snippet (`enabled`, `flow_file`, `mode`) in setup section.
   - Added guardian operations section for `swarm guardian migrate` and `swarm guardian report`.

2. `docs/technical.md`
   - Added guardian config fields to config reference.
   - Added guardian command surface entries (`report`, `migrate`).

3. `docs/release-notes.md`
   - Added 2026-03-06 release section summarizing G5 delivery:
     config parsing, init scaffold, migrate command, and phase chain rule.
   - Captured verification commands executed for G5 completion.

## Scope / intent coverage check

- `g5-01` (guardian config parsing): documented config surface + mode semantics.
- `g5-02` (init scaffold flow file): documented init outputs and default flow file.
- `g5-03` (guardian migrate advisory-first): documented dry-run/apply flows.
- `g5-04` (phase int-gap-tst chain rule): documented in release notes under guardian rule additions.

## Deferred documentation follow-ups

- Add a dedicated Guardian policy authoring page with example `flow.v2.yaml` rule patterns and troubleshooting guidance.
- Add CLI examples showing advisory→enforce rollout sequence for real projects.
