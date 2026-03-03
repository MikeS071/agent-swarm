# Release Notes

## 2026-03-03 — V2-16 Legacy Workflow Deprecation

### Added
- `swarm init` now detects legacy root workflow files (`WORKFLOW_AUTO.md`, `sprint.json`) and archives them to `swarm/archive/legacy-workflow/`.

### Changed
- Legacy workflow-file references were removed from active V2 planning docs.

### Migration Notes
- Existing projects can keep archived copies for historical context.
- New scaffolds will no longer use the legacy workflow-file model.
