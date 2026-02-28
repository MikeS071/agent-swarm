
---
## MANDATORY: Test-Driven Development Process
1. Read the full SPEC.md in the repo root before writing ANY code
2. Write failing tests FIRST that define expected behaviour
3. Implement minimum code to pass tests
4. Quality gates before commit: `go build ./...` && `go test ./...` && `go vet ./...`
5. Commit message format: `feat(TICKET): description`
6. Include in commit body: `Tests: X passing, Y files`
