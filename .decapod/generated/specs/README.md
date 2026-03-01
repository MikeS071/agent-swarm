# Project Specs

Canonical path: `.decapod/generated/specs/`.
These files are the project-local contract for humans and agents.

## Snapshot
- Project: agent-swarm
- Outcome: Go CLI for orchestrating parallel coding agents across isolated git worktrees with dependency tracking and phase gates.
- Detected languages: go
- Detected surfaces: go

## How to use this folder
- `INTENT.md`: what success means and what is explicitly out of scope.
- `ARCHITECTURE.md`: topology, runtime model, data boundaries, and ADR trail.
- `INTERFACES.md`: API/CLI/events/storage contracts and failure behavior.
- `VALIDATION.md`: proof commands, quality gates, and evidence artifacts.
- `SEMANTICS.md`: state machines, invariants, replay rules, and idempotency.
- `OPERATIONS.md`: SLOs, monitoring, incident response, and rollout strategy.
- `SECURITY.md`: threat model, trust boundaries, auth/authz, and supply-chain posture.

## Canonical `.decapod/` Layout
- `.decapod/data/`: canonical control-plane state (SQLite + ledgers).
- `.decapod/generated/specs/`: living project specs for humans and agents.
- `.decapod/generated/context/`: deterministic context capsules.
- `.decapod/generated/policy/context_capsule_policy.json`: repo-native JIT context policy contract.
- `.decapod/generated/artifacts/provenance/`: promotion manifests and convergence checklist.
- `.decapod/generated/artifacts/inventory/`: deterministic release inventory.
- `.decapod/generated/artifacts/diagnostics/`: opt-in diagnostics artifacts.
- `.decapod/workspaces/`: isolated todo-scoped git worktrees.

## Day-0 Onboarding Checklist
- [ ] Replace all placeholders in all 8 spec files.
- [ ] Confirm primary user outcome and acceptance criteria in `INTENT.md`.
- [ ] Confirm topology and runtime model in `ARCHITECTURE.md`.
- [ ] Document all inbound/outbound contracts in `INTERFACES.md`.
- [ ] Define validation gates and CI proof surfaces in `VALIDATION.md`.
- [ ] Define state machines and invariants in `SEMANTICS.md`.
- [ ] Define SLOs, alerting, and incident process in `OPERATIONS.md`.
- [ ] Define threat model and auth/authz decisions in `SECURITY.md`.
- [ ] Ensure architecture diagram, docs, changelog, and tests are mapped to promotion gates.
- [ ] Run all validation/test commands and attach evidence artifacts.

## Agent Directive
- Treat these files as executable governance surfaces. Before implementation: resolve ambiguity and update specs. After implementation: refresh drifted sections, rerun proof gates, and attach evidence.
