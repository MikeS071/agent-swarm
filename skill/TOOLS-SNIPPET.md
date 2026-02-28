## Agent Swarm CLI
- **Binary:** `agent-swarm` — Go CLI for multi-agent orchestration
- **Use for any 3+ ticket parallel build.** Don't manually juggle tmux sessions.
- **Watchdog cron:** every 5min via `swarm/swarm-watchdog.sh` — auto-detects completions, chains spawns
- **Projects registry:** `~/.openclaw/workspace/swarm/projects.json` — add new projects here for watchdog coverage
- **Backend:** Codex tmux (configure model in each project's `swarm.toml`)
