## Agent Swarm (Go CLI — `agent-swarm`)
- **Binary:** `agent-swarm` — Go CLI for multi-agent orchestration with dependency graphs, phase gates, and auto-chaining.
- **Skill:** `agent-swarm` — read SKILL.md for full command reference.
- **Watchdog cron:** runs every 5min, iterates all registered projects, auto-detects completions and chains spawns.
- **Project registry:** `~/.openclaw/workspace/swarm/projects.json` — maps project names to repo paths.

### When to use agent-swarm
- **Any task with 3+ parallel tickets** — don't manually manage tmux sessions
- **Multi-phase builds** — use phase gates for human review between phases
- **Feature branches that need integration** — `agent-swarm integrate` merges in dep order
- **DO NOT use for:** single-file edits, quick fixes, or anything that takes <30 min

### How to use it
1. **New project swarm:**
   ```bash
   cd ~/projects/<name>
   agent-swarm init <name>
   agent-swarm add-ticket <id> --phase 1 --desc "..."
   # Write prompts in swarm/prompts/<id>.md
   agent-swarm watch  # or --once for single pass
   ```
2. **Check status:** `agent-swarm status` (table) or `agent-swarm status --json` (programmatic)
3. **Live TUI:** `agent-swarm status --watch` (bubbletea dashboard with progress bars)
4. **Phase gates:** `agent-swarm go` to approve and auto-spawn next phase
5. **Integration:** `agent-swarm integrate --base main` merges all done branches in dep order
6. **HTTP API:** `agent-swarm serve --port 8090` for web dashboard SSE events
7. **Register project:** add to `~/.openclaw/workspace/swarm/projects.json` for watchdog coverage

### Prompt quality rules
- Each ticket gets `swarm/prompts/<ticket-id>.md` — the agent's full brief
- Include: context, scope, interfaces/types, tests to write first, deliverables
- `swarm/prompt-footer.md` is auto-appended by the watchdog (TDD enforcement)
- **Bad prompt = failed agent.** Invest time here.

### Monitoring
- Watchdog log: `~/.openclaw/workspace/logs/swarm-watchdog.log`
- Project events: `swarm/events.jsonl` in each project dir
- `agent-swarm status --json` for programmatic checks
