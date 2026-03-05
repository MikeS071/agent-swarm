# INT-TP3: Integrate phase 3 guardian + run-scope

## Objective
Integrate guardian spawn enforcement and run-scope post-build architecture.

## Depends
tp-09,tp-10,tp-11,tp-12

## Implementation Steps
1. Wire policy checks + run model in one flow.
2. Validate migration compatibility and live behavior.
3. Add integration tests for enforce mode and run idempotency.

## Verify
go test ./... 

## Done Definition
- phase3 components integrated and stable
