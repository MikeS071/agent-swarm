#!/usr/bin/env bash
set -euo pipefail

CANONICAL_BIN="/home/openclaw/.local/bin/agent-swarm"
REPO_DIR="/home/openclaw/projects/agent-swarm"
SERVICE_FILE="/home/openclaw/.config/systemd/user/swarm-watchdog.service"
SERVICE_NAME="swarm-watchdog.service"
TIMER_NAME="swarm-watchdog.timer"
CHECK_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check-only)
      CHECK_ONLY=1
      shift
      ;;
    *)
      echo "unknown arg: $1" >&2
      exit 2
      ;;
  esac
done

require_bin() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required binary: $1" >&2
    exit 1
  }
}

ensure_service_execstart() {
  [[ -f "$SERVICE_FILE" ]] || {
    echo "service file missing: $SERVICE_FILE" >&2
    exit 1
  }
  local wanted="ExecStart=${CANONICAL_BIN} watchdog run-all-once"
  if grep -qx "$wanted" "$SERVICE_FILE"; then
    return 0
  fi
  local tmp
  tmp=$(mktemp)
  awk -v wanted="$wanted" '
    BEGIN { replaced=0 }
    /^ExecStart=/ {
      if (!replaced) {
        print wanted
        replaced=1
      }
      next
    }
    { print }
  ' "$SERVICE_FILE" > "$tmp"
  mv "$tmp" "$SERVICE_FILE"
}

service_exec_bin() {
  systemctl --user cat "$SERVICE_NAME" \
    | sed -n 's/^ExecStart=//p' \
    | head -n1 \
    | awk '{print $1}'
}

verify_runtime_contract() {
  local cmd_bin service_bin sha_cmd sha_canon version

  cmd_bin="$(command -v agent-swarm)"
  service_bin="$(service_exec_bin)"
  sha_cmd="$(sha256sum "$cmd_bin" | awk '{print $1}')"
  sha_canon="$(sha256sum "$CANONICAL_BIN" | awk '{print $1}')"
  version="$("$CANONICAL_BIN" --version)"

  [[ "$cmd_bin" == "$CANONICAL_BIN" ]] || {
    echo "FAIL: command -v agent-swarm => $cmd_bin (expected $CANONICAL_BIN)" >&2
    exit 1
  }
  [[ "$service_bin" == "$CANONICAL_BIN" ]] || {
    echo "FAIL: $SERVICE_NAME ExecStart uses $service_bin (expected $CANONICAL_BIN)" >&2
    exit 1
  }
  [[ "$sha_cmd" == "$sha_canon" ]] || {
    echo "FAIL: binary hash mismatch between PATH and canonical binary" >&2
    exit 1
  }

  echo "OK: binary=$cmd_bin"
  echo "OK: version=$version"
  echo "OK: service=$SERVICE_NAME exec=$service_bin"
  echo "OK: sha256=$sha_canon"
}

main() {
  require_bin go
  require_bin systemctl
  require_bin sha256sum

  if [[ "$CHECK_ONLY" -eq 0 ]]; then
    (cd "$REPO_DIR" && go build -o "$CANONICAL_BIN" .)
    ensure_service_execstart
    systemctl --user daemon-reload
    systemctl --user restart "$TIMER_NAME"
    "$CANONICAL_BIN" --config "$REPO_DIR/swarm.toml" watchdog run-all-once --dry-run >/dev/null
  fi

  verify_runtime_contract
}

main "$@"
