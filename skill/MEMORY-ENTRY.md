## Agent Swarm CLI
- **agent-swarm Go CLI is the standard tool for multi-ticket parallel builds** (3+ tickets).
- Binary: `agent-swarm` (installed via `go install github.com/MikeS071/agent-swarm@latest`)
- Orchestrates parallel coding agents across isolated git worktrees with dependency graphs and phase gates.
- Commands: init, add-ticket, prompts, watch, status, done, fail, go, integrate, serve, install, cleanup.
- Watchdog auto-detects agent completions, chains dependent tickets, respawns failures, stops at phase gates.
- OpenClaw skill at `skills/agent-swarm/SKILL.md`.
