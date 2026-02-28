#!/usr/bin/env bash
set -euo pipefail

# agent-swarm installer
# Usage: curl -sSL https://raw.githubusercontent.com/MikeS071/agent-swarm/main/install.sh | bash
#    or: bash install.sh [--openclaw]

VERSION="${AGENT_SWARM_VERSION:-latest}"
BINARY_NAME="agent-swarm"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
OPENCLAW_MODE=false

for arg in "$@"; do
    case $arg in
        --openclaw) OPENCLAW_MODE=true ;;
        --help|-h)
            echo "Usage: install.sh [--openclaw]"
            echo "  --openclaw   Also install OpenClaw skill and watchdog cron"
            exit 0
            ;;
    esac
done

echo "🐝 Installing agent-swarm..."

# --- Detect platform ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
esac

# --- Install via go install (preferred) ---
if command -v go &>/dev/null; then
    echo "📦 Installing via go install..."
    go install "github.com/MikeS071/agent-swarm@${VERSION}"
    
    # Find the binary
    GOBIN=$(go env GOBIN 2>/dev/null)
    GOPATH=$(go env GOPATH 2>/dev/null)
    if [ -z "$GOBIN" ]; then
        GOBIN="${GOPATH:-$HOME/go}/bin"
    fi
    
    if [ -f "$GOBIN/agent-swarm" ]; then
        mkdir -p "$INSTALL_DIR"
        cp "$GOBIN/agent-swarm" "$INSTALL_DIR/$BINARY_NAME"
        echo "✅ Installed to $INSTALL_DIR/$BINARY_NAME"
    else
        echo "⚠️  go install succeeded but binary not found at $GOBIN/agent-swarm"
        echo "   Make sure \$GOBIN or \$GOPATH/bin is in your PATH"
    fi
else
    echo "❌ Go is required. Install Go first: https://go.dev/dl/"
    echo "   Then run: go install github.com/MikeS071/agent-swarm@latest"
    exit 1
fi

# --- Verify ---
if command -v "$BINARY_NAME" &>/dev/null; then
    echo "✅ agent-swarm is in PATH"
    "$BINARY_NAME" status --help 2>/dev/null | head -1 || true
elif [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo "⚠️  Installed at $INSTALL_DIR/$BINARY_NAME but not in PATH"
    echo "   Add to PATH: export PATH=\"$INSTALL_DIR:\$PATH\""
fi

# --- Check tmux ---
if ! command -v tmux &>/dev/null; then
    echo "⚠️  tmux not found — required for codex-tmux backend"
    echo "   Install: apt install tmux  or  brew install tmux"
fi

# --- Check codex ---
if ! command -v codex &>/dev/null; then
    echo "⚠️  codex not found — required for codex-tmux backend"
    echo "   Install: npm install -g @anthropic-ai/codex"
fi

# --- OpenClaw integration ---
if [ "$OPENCLAW_MODE" = true ]; then
    echo ""
    echo "🔧 Setting up OpenClaw integration..."
    
    OPENCLAW_WORKSPACE="${OPENCLAW_WORKSPACE:-$HOME/.openclaw/workspace}"
    SKILL_DIR="$OPENCLAW_WORKSPACE/skills/agent-swarm"
    
    # Install skill
    mkdir -p "$SKILL_DIR"
    # Copy all skill files
    for SKILL_FILE in SKILL.md AGENTS-SNIPPET.md TOOLS-SNIPPET.md MEMORY-ENTRY.md; do
        if [ -f "skill/$SKILL_FILE" ]; then
            cp "skill/$SKILL_FILE" "$SKILL_DIR/$SKILL_FILE"
        else
            curl -sSL "https://raw.githubusercontent.com/MikeS071/agent-swarm/main/skill/$SKILL_FILE" \
                -o "$SKILL_DIR/$SKILL_FILE" 2>/dev/null || true
        fi
    done
    echo "✅ Skill files installed at $SKILL_DIR/"
    
    # Create swarm workspace directory
    SWARM_DIR="$OPENCLAW_WORKSPACE/swarm"
    mkdir -p "$SWARM_DIR/prompts"
    
    # Create projects.json if it doesn't exist
    if [ ! -f "$SWARM_DIR/projects.json" ]; then
        echo '{}' > "$SWARM_DIR/projects.json"
        echo "✅ Created $SWARM_DIR/projects.json"
    fi
    
    # Create prompt footer template
    if [ ! -f "$SWARM_DIR/prompt-footer.md" ]; then
        cat > "$SWARM_DIR/prompt-footer.md" << 'FOOTER'

---
## MANDATORY: Test-Driven Development Process
1. Read any SPEC.md or README in the repo root before writing code
2. Write failing tests FIRST that define expected behaviour
3. Implement minimum code to pass tests
4. Quality gates before commit: build passes, tests pass, lint clean
5. Commit message format: `feat(TICKET): description`
6. Include in commit body: `Tests: X passing, Y files`
FOOTER
        echo "✅ Created prompt footer template"
    fi
    
    # Create watchdog wrapper script
    cat > "$SWARM_DIR/swarm-watchdog.sh" << 'WATCHDOG'
#!/usr/bin/env bash
# agent-swarm watchdog for OpenClaw — runs one pass across all registered projects
set -euo pipefail

SWARM_DIR="$(dirname "$(realpath "$0")")"
PROJECTS_JSON="$SWARM_DIR/projects.json"
LOG_DIR="${OPENCLAW_WORKSPACE:-$HOME/.openclaw/workspace}/logs"
mkdir -p "$LOG_DIR"

# Run watchdog for each registered project that has a swarm.toml
for PROJECT_DIR in $(python3 -c "
import json, os
p = json.load(open('$PROJECTS_JSON'))
for name, cfg in p.items():
    repo = cfg.get('repo', '')
    if os.path.isfile(os.path.join(repo, 'swarm.toml')):
        print(repo)
" 2>/dev/null); do
    echo "$(date -u +%FT%TZ) Checking $PROJECT_DIR"
    cd "$PROJECT_DIR"
    agent-swarm watch --once 2>&1 || true
done
WATCHDOG
    chmod +x "$SWARM_DIR/swarm-watchdog.sh"
    echo "✅ Created watchdog wrapper at $SWARM_DIR/swarm-watchdog.sh"
    
    # Install cron
    CRON_CMD="*/5 * * * * bash $SWARM_DIR/swarm-watchdog.sh >> $LOG_DIR/swarm-watchdog.log 2>&1"
    if crontab -l 2>/dev/null | grep -qF "swarm-watchdog.sh"; then
        echo "ℹ️  Watchdog cron already installed"
    else
        (crontab -l 2>/dev/null; echo "$CRON_CMD") | crontab -
        echo "✅ Watchdog cron installed (every 5 minutes)"
    fi
    
    # --- Inject workspace file snippets ---
    SKILL_REPO_DIR="$SKILL_DIR"
    
    # AGENTS.md — append swarm section if not already present
    AGENTS_FILE="$OPENCLAW_WORKSPACE/AGENTS.md"
    if [ -f "$AGENTS_FILE" ]; then
        if ! grep -q "Agent Swarm (Go CLI" "$AGENTS_FILE" 2>/dev/null; then
            echo "" >> "$AGENTS_FILE"
            cat "$SKILL_DIR/AGENTS-SNIPPET.md" >> "$AGENTS_FILE"
            echo "✅ Appended agent-swarm section to AGENTS.md"
        else
            echo "ℹ️  AGENTS.md already has agent-swarm section"
        fi
    else
        echo "⚠️  No AGENTS.md found at $AGENTS_FILE — create one and add the agent-swarm section from skill/AGENTS-SNIPPET.md"
    fi
    
    # TOOLS.md — append swarm section if not already present
    TOOLS_FILE="$OPENCLAW_WORKSPACE/TOOLS.md"
    if [ -f "$TOOLS_FILE" ]; then
        if ! grep -q "Agent Swarm CLI" "$TOOLS_FILE" 2>/dev/null; then
            echo "" >> "$TOOLS_FILE"
            cat "$SKILL_DIR/TOOLS-SNIPPET.md" >> "$TOOLS_FILE"
            echo "✅ Appended agent-swarm section to TOOLS.md"
        else
            echo "ℹ️  TOOLS.md already has agent-swarm section"
        fi
    else
        echo "⚠️  No TOOLS.md found at $TOOLS_FILE — create one and add from skill/TOOLS-SNIPPET.md"
    fi
    
    # MEMORY.md — append entry if not already present
    MEMORY_FILE="$OPENCLAW_WORKSPACE/MEMORY.md"
    if [ -f "$MEMORY_FILE" ]; then
        if ! grep -q "agent-swarm Go CLI is the standard tool" "$MEMORY_FILE" 2>/dev/null; then
            echo "" >> "$MEMORY_FILE"
            cat "$SKILL_DIR/MEMORY-ENTRY.md" >> "$MEMORY_FILE"
            echo "✅ Appended agent-swarm entry to MEMORY.md"
        else
            echo "ℹ️  MEMORY.md already has agent-swarm entry"
        fi
    else
        echo "⚠️  No MEMORY.md found — create one and add from skill/MEMORY-ENTRY.md"
    fi
    
    echo ""
    echo "🎉 OpenClaw integration complete!"
    echo ""
    echo "To register a project:"
    echo "  cd ~/projects/my-app"
    echo "  agent-swarm init my-app"
    echo ""
    echo "  # Add to OpenClaw project registry:"
    echo "  python3 -c \""
    echo "  import json"
    echo "  p = json.load(open('$SWARM_DIR/projects.json'))"
    echo "  p['my-app'] = {'repo': '$HOME/projects/my-app', 'tracker': 'swarm/tracker.json', 'promptDir': 'swarm/prompts'}"
    echo "  json.dump(p, open('$SWARM_DIR/projects.json','w'), indent=2)"
    echo "  \""
fi

echo ""
echo "🐝 agent-swarm installed successfully!"
echo ""
echo "Get started:"
echo "  cd ~/projects/my-app"
echo "  agent-swarm init my-app"
echo "  agent-swarm add-ticket feat-01 --phase 1 --desc 'First feature'"
echo "  agent-swarm prompts gen feat-01"
echo "  agent-swarm watch"
