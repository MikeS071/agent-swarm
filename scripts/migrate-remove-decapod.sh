#!/usr/bin/env bash
set -euo pipefail

TARGET_REPO="${1:-.}"

if [[ ! -d "$TARGET_REPO" ]]; then
  echo "Target repo path does not exist: $TARGET_REPO" >&2
  exit 1
fi

TARGET_REPO="$(cd "$TARGET_REPO" && pwd)"

log() {
  printf '%s\n' "$*"
}

clean_decapod_lines() {
  local file="$1"
  local tmp
  tmp="$(mktemp)"

  awk '
    BEGIN { IGNORECASE = 1 }
    {
      if ($0 ~ /decapod/ || $0 ~ /agent\.init/ || $0 ~ /proof\.validate/) {
        next
      }
      print
    }
  ' "$file" >"$tmp"

  if cmp -s "$file" "$tmp"; then
    rm -f "$tmp"
    return
  fi

  mv "$tmp" "$file"
  log "Updated $file"
}

DECAPOD_BIN="${HOME}/.cargo/bin/decapod"
if [[ -e "$DECAPOD_BIN" ]]; then
  rm -f "$DECAPOD_BIN"
  log "Removed $DECAPOD_BIN"
fi

while IFS= read -r -d '' dir; do
  rm -rf "$dir"
  log "Removed $dir"
done < <(find "$TARGET_REPO" -type d -name ".decapod" -print0)

PROMPT_FOOTER="$TARGET_REPO/swarm/prompt-footer.md"
if [[ -f "$PROMPT_FOOTER" ]]; then
  clean_decapod_lines "$PROMPT_FOOTER"
fi

while IFS= read -r -d '' codex; do
  clean_decapod_lines "$codex"
done < <(find "$TARGET_REPO" -type f -name "CODEX.md" -print0)

TEMPLATE_AGENTS="${HOME}/.openclaw/workspace/templates/AGENTS.md"
if [[ -f "$TEMPLATE_AGENTS" ]]; then
  clean_decapod_lines "$TEMPLATE_AGENTS"
fi

log "Decapod migration cleanup complete."
