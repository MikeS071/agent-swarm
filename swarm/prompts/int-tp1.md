# INT-TP1: Integrate phase 1 ticket prep core

## Objective
Integrate tracker schema v2, tickets lint, prompts build, and prompts validate into a coherent flow.

## Depends
tp-01, tp-02, tp-03, tp-04

## Files to touch
- cmd wiring / shared helpers
- docs snippets if needed
- integration tests

## Implementation Steps
1. Wire command flow: lint -> build -> validate.
2. Ensure clear failure messages and stable exit behavior.
3. Add integration tests for all-phase1 happy path + failure path.

## Verify
go test ./cmd/... ./internal/...

## Done Definition
- single repo state supports complete phase1 workflow
- tests cover successful and failing pipelines
