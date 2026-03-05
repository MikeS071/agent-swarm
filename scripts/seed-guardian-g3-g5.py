#!/usr/bin/env python3
import json
from pathlib import Path

repo = Path('/home/openclaw/projects/agent-swarm')
tracker_path = repo / '.local/state/tracker.json'
prompt_dir = repo / 'swarm/prompts'
prompt_dir.mkdir(parents=True, exist_ok=True)

def ticket(tid, phase, deps, desc, profile, verify, ttype=''):
    body = {
        'status': 'todo',
        'phase': phase,
        'depends': deps,
        'branch': f'feat/{tid}',
        'desc': f'{desc} Scope: {desc} Verify: `{verify}`',
        'profile': profile,
        'verify_cmd': verify,
    }
    if ttype:
        body['type'] = ttype
        body['feature'] = 'run'
    return body

# Guardian delta (G3-G5) + doc-only post_build seeded upfront.
base = {
    'g3-01': ticket('g3-01', 1, [], 'Enforce guardian checks before ticket spawn.', 'code-agent', 'go test ./internal/watchdog/... -run GuardianSpawn'),
    'g3-02': ticket('g3-02', 1, ['g3-01'], 'Enforce guardian checks before marking tickets done.', 'code-agent', 'go test ./internal/watchdog/... -run GuardianDone'),
    'g3-03': ticket('g3-03', 1, ['g3-01'], 'Implement Guardian rules for ticket scope/verify and prompt-section checks.', 'code-agent', "go test ./internal/guardian/rules/... -run 'Ticket|Prompt'"),
    'int-g3': ticket('int-g3', 1, ['g3-01', 'g3-02', 'g3-03'], 'Integrate Guardian spawn/done enforcement paths.', 'code-agent', 'go test ./... -count=1'),
    'tst-g3': ticket('tst-g3', 1, ['int-g3'], 'End-to-end verification for spawn/done enforcement.', 'e2e-runner', 'go test ./... -count=1'),

    'g4-01': ticket('g4-01', 2, ['tst-g3'], 'Implement approvals store and audit trail in guardian evidence layer.', 'code-agent', 'go test ./internal/guardian/evidence/... -run Approval'),
    'g4-02': ticket('g4-02', 2, ['g4-01'], 'Capture machine-readable evidence payloads for guardian decisions.', 'code-agent', 'go test ./internal/guardian/... -run Evidence'),
    'g4-03': ticket('g4-03', 2, ['g4-02'], 'Add guardian report command with reasons and evidence paths.', 'code-agent', 'go test ./cmd/... -run GuardianReport'),
    'int-g4': ticket('int-g4', 2, ['g4-01', 'g4-02', 'g4-03'], 'Integrate approvals/evidence/report into runtime flow.', 'code-agent', 'go test ./... -count=1'),
    'tst-g4': ticket('tst-g4', 2, ['int-g4'], 'Verification matrix for evidence + reporting flows.', 'e2e-runner', 'go test ./... -count=1'),

    'g5-01': ticket('g5-01', 3, ['tst-g4'], 'Add guardian config parsing (enabled/flow_file/mode).', 'code-agent', 'go test ./internal/config/... -run Guardian'),
    'g5-02': ticket('g5-02', 3, ['g5-01'], 'Scaffold default swarm/flow.v2.yaml on init.', 'code-agent', 'go test ./cmd/... -run Init'),
    'g5-03': ticket('g5-03', 3, ['g5-02'], 'Implement guardian migrate command (advisory-first).', 'code-agent', 'go test ./cmd/... -run GuardianMigrate'),
    'g5-04': ticket('g5-04', 3, ['g5-01'], 'Implement phase_has_int_gap_tst_chain guardian rule.', 'code-agent', 'go test ./internal/guardian/rules/... -run Chain'),
    'int-g5': ticket('int-g5', 3, ['g5-01', 'g5-02', 'g5-03', 'g5-04'], 'Final integration for config/init/migrate/rules stack.', 'code-agent', 'go test ./... -count=1'),
    'tst-g5': ticket('tst-g5', 3, ['int-g5'], 'Final guardian verification in advisory and enforce modes.', 'e2e-runner', 'go test ./... -count=1'),
}

# doc-run depends on all non-post-build tickets to stay visible from start but run last.
doc_deps = sorted(base.keys())
base['doc-run'] = ticket('doc-run', 4, doc_deps, 'Documentation update for Guardian G3-G5 completion state.', 'doc-updater', 'go build ./...', ttype='doc')

tracker = {'project': 'agent-swarm', 'tickets': base, 'unlocked_phase': 1}
tracker_path.write_text(json.dumps(tracker, indent=2) + '\n')

for tid, tk in base.items():
    deps = tk.get('depends', [])
    deps_text = 'none' if not deps else ', '.join(deps)

    if tk.get('type') == 'doc':
        prompt = f"""# {tid}

## Objective
{tk['desc'].split(' Scope:')[0]}

## Dependencies
{deps_text}

## Scope
- Update user-facing docs (`docs/user-guide.md`) where behavior changed.
- Update technical docs (`docs/technical.md`) with architecture/flow changes.
- Update release notes (`docs/release-notes.md`) for shipped impact.
- If no doc updates are required, provide explicit no-op justification.

## Inputs
- swarm/features/run/review-report.json
- swarm/features/run/sec-report.json
- swarm/features/run/gap-report.md

## Required deliverable
Always produce and commit:
- swarm/features/run/doc-report.md

## Verify
{tk['verify_cmd']}
"""
    else:
        prompt = f"""# {tid}

## Objective
{tk['desc'].split(' Scope:')[0]}

## Dependencies
{deps_text}

## Scope
- Implement only what is required for `{tid}`.
- Keep changes focused and reversible.
- Update/add tests first where applicable.

## Verify
{tk['verify_cmd']}
"""

    (prompt_dir / f'{tid}.md').write_text(prompt)

print(f'seeded_tickets={len(base)}')
