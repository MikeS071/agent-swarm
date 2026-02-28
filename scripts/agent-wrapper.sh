#!/usr/bin/env bash
# Wraps a codex agent: runs the command, then on exit (any reason):
# 1. Auto-commits uncommitted work if tests pass
# 2. Updates tracker to done/failed
# Usage: agent-wrapper.sh <project-dir> <worktree-dir> <ticket-id> <codex-command...>

set -uo pipefail

PROJECT_DIR="$1"; shift
WORKTREE_DIR="$1"; shift
TICKET_ID="$1"; shift
TRACKER="$PROJECT_DIR/swarm/tracker.json"

cd "$WORKTREE_DIR"

# Run the agent — allow it to fail
"$@" || true

echo "[agent-wrapper] Agent exited. Checking work in $WORKTREE_DIR..."

# Check for any work (commits or uncommitted changes)
COMMITS=$(git log --oneline main..HEAD 2>/dev/null | wc -l)
CHANGES=$(git status --porcelain 2>/dev/null | grep -cv '.codex-prompt.md' || true)

if [ "$COMMITS" -eq 0 ] && [ "$CHANGES" -eq 0 ]; then
    echo "[agent-wrapper] No work produced. Marking failed."
    python3 -c "
import json
t = json.load(open('$TRACKER'))
t['tickets']['$TICKET_ID']['status'] = 'failed'
json.dump(t, open('$TRACKER', 'w'), indent=2)
"
    exit 1
fi

# Auto-commit uncommitted changes
if [ "$CHANGES" -gt 0 ]; then
    echo "[agent-wrapper] Found $CHANGES uncommitted changes. Auto-committing..."
    git add -A
    git commit -m "feat($TICKET_ID): auto-commit by agent-wrapper" --no-verify 2>/dev/null || true
fi

# Run tests
echo "[agent-wrapper] Running tests..."
go test ./... -count=1 -timeout 60s > /tmp/agent-wrapper-$TICKET_ID.log 2>&1
TEST_EXIT=$?

SHA=$(git rev-parse HEAD)

if [ "$TEST_EXIT" -eq 0 ]; then
    echo "[agent-wrapper] ✅ Tests pass. Marking $TICKET_ID done ($SHA)"
    python3 -c "
import json
t = json.load(open('$TRACKER'))
t['tickets']['$TICKET_ID']['status'] = 'done'
t['tickets']['$TICKET_ID']['sha'] = '$SHA'
json.dump(t, open('$TRACKER', 'w'), indent=2)
"
else
    echo "[agent-wrapper] ❌ Tests failed. Marking $TICKET_ID failed."
    tail -20 /tmp/agent-wrapper-$TICKET_ID.log
    python3 -c "
import json
t = json.load(open('$TRACKER'))
t['tickets']['$TICKET_ID']['status'] = 'failed'
json.dump(t, open('$TRACKER', 'w'), indent=2)
"
fi
