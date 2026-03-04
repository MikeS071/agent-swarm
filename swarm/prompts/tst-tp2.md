# TST-TP2: Verify prep + role context + spawn binding

## Objective
Test phase2 end-to-end behavior in strict mode.

## Depends
int-tp2

## Implementation Steps
1. Fixture project from init with agentkit.
2. Run prep strict.
3. Spawn sample ticket and assert .agent-context + manifest present.
4. Negative tests for missing role files.

## Verify
go test ./... 

## Done Definition
- verified strict gating + context binding reliability
