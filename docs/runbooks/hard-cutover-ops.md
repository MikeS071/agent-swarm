# Hard-Cutover Ops Runbook (Agent-Swarm + Guardian)

## Pre-launch (mandatory)

1. Validate gates:
```bash
swarm doctor --json
```

2. Run strict preflight:
```bash
swarm prep --json
```

3. If both pass, execute one control pass:
```bash
swarm watch --once
```

4. Start looped orchestration only after control pass is clean:
```bash
swarm watch
```

---

## Hard gates currently enforced

- Explicit ticket role/profile required
- Verify command required per ticket (or integration fallback)
- Prompt file must exist for ticket spawn
- Guardian blocking checks at:
  - `before_spawn`
  - `before_mark_done`
  - `phase_transition`
  - `post_build_complete`
- Completion requires meaningful code changes + verify pass

---

## Failure triage checklist

### A) Spawn blocked
1. Run:
```bash
swarm doctor --json
swarm prep --json
```
2. Inspect latest events:
```bash
tail -200 .local/state/events.jsonl  # or <state_dir>/events.jsonl
```
3. Look for `guardian_block` and reason/rule/evidence.
4. Run `swarm status --json` (it reconciles dead running sessions).

### B) Ticket fails verification
1. Check ticket verify command in tracker (`verify_cmd`).
2. Reproduce in worktree manually.
3. Fix command or code; respawn ticket.

### C) Phase transition blocked
1. Find `guardian_block` with `event=phase_transition`.
2. Resolve required policy artifacts/evidence.
3. Re-run `watch --once`.

### D) Completion blocked
1. Find `guardian_block` with `event=post_build_complete`.
2. Resolve post-build policy requirements.
3. Re-run `watch --once`.

---

## Telemetry + retention

- Raw stream: `events.jsonl`
- Retention: keep last 30 days (automatic prune)
- Daily summaries: `rollups/YYYY-MM-DD.json`

---

## One-command launch readiness

Use this before every run:
```bash
swarm doctor
```

If it fails, do not start `swarm watch`.
