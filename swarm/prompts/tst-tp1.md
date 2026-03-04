# TST-TP1: Verify phase 1 end-to-end

## Objective
Execute end-to-end verification for phase 1 core workflow.

## Depends
int-tp1

## Implementation Steps
1. Create fixture tracker with valid schema v2 tickets.
2. Run tickets lint strict, prompts build all, prompts validate strict.
3. Add negative fixtures for missing fields/placeholders.
4. Assert expected exit/failure behavior.

## Verify
go test ./... 

## Done Definition
- E2E tests demonstrate gate quality for phase 1
