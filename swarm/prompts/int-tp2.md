# INT-TP2: Integrate phase 2 agentkit + preflight

## Objective
Integrate prep gating, agentkit scaffolding, role checks, and context binding.

## Depends
tp-05,tp-06,tp-07,tp-08

## Implementation Steps
1. Ensure prep fails when role context unresolved.
2. Ensure watch gating and spawn context binding work together.
3. Add integration tests spanning init->prep->spawn.

## Verify
go test ./... 

## Done Definition
- phase2 workflow stable and tested end-to-end
