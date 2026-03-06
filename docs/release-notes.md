# Release Notes

## 2026-03-06 — Guardian G5 Delivery (Config + Init + Migrate + Chain Rule)

### Added
- Guardian config parsing now supports:
  - `guardian.enabled`
  - `guardian.flow_file`
  - `guardian.mode` (`advisory` | `enforce`)
- `swarm init` now scaffolds default `swarm/flow.v2.yaml` guardian flow file.
- New `swarm guardian migrate` command:
  - default dry-run report
  - `--apply` writes advisory-first safe defaults and scaffolds missing flow file
- New phase rule helper `CheckPhaseIntGapTstChain` validating int→gap→tst dependency chain per phase.

### Changed
- Guardian config loading now normalizes mode values and validates allowed values.
- Documentation updated for guardian migration/reporting workflows.

### Validation
- `go test ./internal/config/... -run Guardian`
- `go test ./cmd/... -run Init`
- `go test ./cmd/... -run GuardianMigrate`
- `go test ./internal/guardian/rules/... -run Chain`
- `go test ./... -count=1`

## 2026-03-05 — Post-build Simplification + Runtime Refresh Hardening

### Added
- `scripts/refresh-runtime.sh` deterministic runtime refresh script:
  - rebuild canonical binary (`~/.local/bin/agent-swarm`)
  - enforce watchdog service ExecStart path
  - reload/restart user systemd timer
  - verify path/version/hash parity

### Changed
- Default post-build pipeline is now doc-only (`post_build.order = ["doc"]`).
- Post-build execution is gated on integrated base branch (`dev`) before spawn.

### Operational Notes
- Existing runs should remove non-doc post-build tickets from active tracker if migrating mid-run.
- If `dev` diverges, resolve merge conflicts in integration path before post-build spawn.

## 2026-03-03 — V2-16 Legacy Workflow Deprecation

### Added
- `swarm init` now detects legacy root workflow files (`WORKFLOW_AUTO.md`, `sprint.json`) and archives them to `swarm/archive/legacy-workflow/`.

### Changed
- Legacy workflow-file references were removed from active V2 planning docs.

### Migration Notes
- Existing projects can keep archived copies for historical context.
- New scaffolds will no longer use the legacy workflow-file model.
