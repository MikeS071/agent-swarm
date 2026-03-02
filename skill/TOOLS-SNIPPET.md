## Agent Swarm CLI
- **Binary:** `agent-swarm` — Go CLI for multi-agent orchestration
- **Use for any 3+ ticket parallel build.** Don't manually juggle tmux sessions.
- **TUI dashboard:** `agent-swarm watch` — live status, keybindings: `A`=approve gate, `m`=toggle auto/manual, `k`=kill, `r`=respawn, `p`=switch project
- **Phase gates:** manual (default) or auto mode — toggle with `m` in TUI or `auto_approve` in swarm.toml
- **Backend:** Codex tmux (configure model in each project's `swarm.toml`)
- **Worktrees:** agents run in `<repo>-worktrees/<ticket>/` (outside project dir)
