# TST-TP3: Final verification matrix for Ticket Prep V2

## Objective
Run full verification matrix covering lint/build/validate/prep/guardian/run-scope.

## Depends
int-tp3

## Implementation Steps
1. Run full test suite.
2. Validate prep gate blocks unprepared tickets.
3. Validate guardian before_spawn enforce blocks invalid prompts.
4. Validate post-build steps run once per run.
5. Capture regression notes.

## Verify
go test ./... 

## Done Definition
- all Ticket Prep V2 acceptance criteria are demonstrably met
