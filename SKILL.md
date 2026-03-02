# Agent Swarm — OpenClaw Skill

CLI tool for orchestrating parallel coding agents across isolated git worktrees.

## Install

```bash
go install github.com/MikeS071/agent-swarm@latest
```

Binary: `agent-swarm`

## Quick Start

```bash
cd ~/projects/my-project
agent-swarm init                    # create swarm/ directory + swarm.toml
agent-swarm add-ticket T-01 ...     # add tickets
agent-swarm watch                   # start TUI + watchdog
```

## Commands

| Command | Description |
|---------|-------------|
| `agent-swarm init` | Scaffold swarm directory with config and tracker |
| `agent-swarm status` | Show tracker status (table, JSON, or TUI) |
| `agent-swarm status --watch` | Live TUI dashboard |
| `agent-swarm watch` | Start watchdog + TUI (spawns/monitors agents) |
| `agent-swarm done <ticket> [sha]` | Mark ticket done |
| `agent-swarm fail <ticket>` | Mark ticket failed |
| `agent-swarm go` | Approve phase gate (CLI equivalent of TUI `A`) |
| `agent-swarm archive` | Archive done tickets to `swarm/archive.json` |
| `agent-swarm archive restore` | Restore archived tickets |
| `agent-swarm archive list` | List archived tickets |
| `agent-swarm prompts` | Generate prompt files from tracker |
| `agent-swarm integrate` | Merge feature branches in dependency order |
| `agent-swarm cleanup` | Remove completed worktrees |

## TUI Keybindings

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate tickets |
| `Enter` | View agent output |
| `Esc` | Back to list |
| `k` | Kill selected agent |
| `r` | Respawn selected ticket |
| `A` | Approve phase gate |
| `m` | Toggle auto/manual mode (persists to swarm.toml) |
| `p` | Cycle through projects |
| `[`/`]` | Previous/next page |
| `Tab` | Toggle compact view |
| `q` | Quit |

Title bar shows `[auto]` or `[manual]`. Auto mode skips phase gates.

## Config (`swarm.toml`)

```toml
[project]
name = "my-project"
repo = "/absolute/path/to/repo"
base_branch = "main"
max_agents = 5
auto_approve = false    # toggle with 'm' in TUI

[watchdog]
interval = "30s"
max_runtime = "60m"
max_retries = 2

[notifications]
telegram_token_cmd = "pass show apis/telegram-bot-token"
telegram_chat_id = "123456789"
```

## Tracker (`swarm/tracker.json`)

```json
{
  "tickets": {
    "T-01": {
      "status": "todo",
      "phase": 1,
      "depends": [],
      "branch": "feat/T-01",
      "desc": "Ticket description"
    }
  }
}
```

Statuses: `todo` → `running` → `done` | `failed`

## Phase Gates

Tickets within a phase run in parallel. Phase N+1 starts only after Phase N completes.

- **Manual mode** (`auto_approve = false`): Watchdog stops at each gate. Press `A` in TUI or run `agent-swarm go` to advance.
- **Auto mode** (`auto_approve = true`): Phases advance automatically. Toggle at runtime with `m`.

## Prompts

Place prompt files in `swarm/prompts/<ticket-id>.md`. The watchdog reads the prompt when spawning an agent for that ticket.

## Worktrees

Each agent gets an isolated git worktree at `<repo>-worktrees/<ticket-id>/`. Worktrees are outside the project directory to avoid IDE/bundler scanning issues.

## Multi-Project

The TUI auto-discovers projects by scanning for `swarm.toml` files (depth 3). Press `p` to cycle between projects.

## Agent Backend

Currently supports Codex CLI via tmux sessions. Each agent runs in `swarm-<ticket-id>` tmux session.
