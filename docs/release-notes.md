# Release Notes

## 2026-03-05 — Run Control Documentation Update

### Added
- User guide instructions for triggering a single watchdog pass via `swarm watch --once` and `POST /api/watchdog/run`.
- API auth usage notes showing bearer-token requirements when `serve.auth_token` is configured.

### Changed
- Technical docs now describe all run execution paths (CLI loop, CLI single-pass, HTTP-triggered single pass).
- Technical API notes now include concrete watchdog run endpoint response semantics (`202`, `503`, `500`, auth `401`).

## 2026-03-03 — V2-16 Legacy Workflow Deprecation

### Added
- `swarm init` now detects legacy root workflow files (`WORKFLOW_AUTO.md`, `sprint.json`) and archives them to `swarm/archive/legacy-workflow/`.

### Changed
- Legacy workflow-file references were removed from active V2 planning docs.

### Migration Notes
- Existing projects can keep archived copies for historical context.
- New scaffolds will no longer use the legacy workflow-file model.
