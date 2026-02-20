#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: vault_init.sh [--path <dir>]

Initialize an Obsidian-compatible vault structure.

Path resolution order:
1) --path <dir>
2) OBSIDIAN_VAULT_PATH
3) ~/.agents/vault.conf (first non-empty non-comment line)
4) /project/obsidian-vault

By default, run-scoped /workspace paths are rejected for durable vault storage.
Set OBSIDIAN_ALLOW_WORKSPACE_PATH=1 to force an override.
USAGE
}

resolve_vault_candidate() {
  local explicit="${1:-}"
  if [ -n "$explicit" ]; then
    printf '%s\n' "$explicit"
    return
  fi

  if [ -n "${OBSIDIAN_VAULT_PATH:-}" ]; then
    printf '%s\n' "$OBSIDIAN_VAULT_PATH"
    return
  fi

  local conf="$HOME/.agents/vault.conf"
  if [ -f "$conf" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      line="$(printf '%s' "$line" | sed 's/^\s*//;s/\s*$//')"
      [ -z "$line" ] && continue
      case "$line" in
        \#*) continue ;;
      esac
      printf '%s\n' "$line"
      return
    done < "$conf"
  fi

  printf '%s\n' "/project/obsidian-vault"
}

to_host_path() {
  local path_in="$1"
  if [ -z "$path_in" ]; then
    printf '%s\n' "$path_in"
    return
  fi
  case "$path_in" in
    /project)
      printf '%s\n' "$PWD"
      return
      ;;
    /project/*)
      if [ -d "/project" ]; then
        printf '%s\n' "$path_in"
      else
        printf '%s/%s\n' "$PWD" "${path_in#/project/}"
      fi
      return
      ;;
  esac
  printf '%s\n' "$path_in"
}

reject_workspace_path() {
  local logical="$1"
  local resolved="$2"
  if [ "${OBSIDIAN_ALLOW_WORKSPACE_PATH:-0}" = "1" ]; then
    return 0
  fi
  case "$logical" in
    /workspace|/workspace/*)
      echo "ERROR: refusing run-scoped /workspace path for vault storage: $logical" >&2
      echo "Set OBSIDIAN_ALLOW_WORKSPACE_PATH=1 to force this override." >&2
      exit 1
      ;;
  esac
  case "$resolved" in
    /workspace|/workspace/*)
      echo "ERROR: refusing run-scoped /workspace path for vault storage: $resolved" >&2
      echo "Set OBSIDIAN_ALLOW_WORKSPACE_PATH=1 to force this override." >&2
      exit 1
      ;;
  esac
}

VAULT_PATH=""
while [ $# -gt 0 ]; do
  case "$1" in
    --path)
      [ $# -ge 2 ] || { echo "ERROR: --path requires a value" >&2; exit 1; }
      VAULT_PATH="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

VAULT_PATH_LOGICAL="$(resolve_vault_candidate "$VAULT_PATH")"
VAULT_PATH="$(to_host_path "$VAULT_PATH_LOGICAL")"
reject_workspace_path "$VAULT_PATH_LOGICAL" "$VAULT_PATH"
mkdir -p "$VAULT_PATH"

created_dirs=()
existing_dirs=()
created_files=()
existing_files=()

ensure_dir() {
  local d="$1"
  if [ -d "$d" ]; then
    existing_dirs+=("$d")
  else
    mkdir -p "$d"
    created_dirs+=("$d")
  fi
}

ensure_file() {
  local f="$1"
  local content="$2"
  if [ -f "$f" ]; then
    existing_files+=("$f")
  else
    mkdir -p "$(dirname "$f")"
    printf '%s\n' "$content" > "$f"
    created_files+=("$f")
  fi
}

ensure_dir "$VAULT_PATH/inbox"
ensure_dir "$VAULT_PATH/notes"
ensure_dir "$VAULT_PATH/mocs"
ensure_dir "$VAULT_PATH/journals"
ensure_dir "$VAULT_PATH/templates"
ensure_dir "$VAULT_PATH/.obsidian"

ensure_file "$VAULT_PATH/templates/fleeting.md" "---
type: F
title: \"\"
created: \"\"
tags: []
up: \"\"
---

- Quick capture
"

ensure_file "$VAULT_PATH/templates/literature.md" "---
type: L
title: \"\"
created: \"\"
source: \"\"
tags: []
up: \"\"
---

## Summary

## Key claims

## Quotes
"

ensure_file "$VAULT_PATH/templates/permanent.md" "---
type: P
title: \"\"
created: \"\"
tags: []
aliases: []
up: \"\"
---

## Idea

## Why it matters

## Linked notes
"

ensure_file "$VAULT_PATH/templates/moc.md" "---
type: MOC
title: \"\"
created: \"\"
tags: []
---

## Index
"

ensure_file "$VAULT_PATH/templates/journal.md" "---
type: JOURNAL
title: \"\"
created: \"\"
tags: []
---

## Session goals

## Work log

## Next actions
"

ensure_file "$VAULT_PATH/mocs/index.md" "---
type: MOC
title: \"Knowledge Index\"
created: \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
tags: [index]
---

# Knowledge Index

- [[mocs/index]]
"

ensure_file "$VAULT_PATH/.obsidian/app.json" "{\"legacyEditor\": false, \"showInlineTitle\": true}"

join_arr() {
  local out=""
  local item
  for item in "$@"; do
    if [ -z "$out" ]; then
      out="$item"
    else
      out="$out"$'\x1f'"$item"
    fi
  done
  printf '%s' "$out"
}

export _VAULT_CREATED_DIRS="$(join_arr "${created_dirs[@]:-}")"
export _VAULT_EXISTING_DIRS="$(join_arr "${existing_dirs[@]:-}")"
export _VAULT_CREATED_FILES="$(join_arr "${created_files[@]:-}")"
export _VAULT_EXISTING_FILES="$(join_arr "${existing_files[@]:-}")"

python3 - "$VAULT_PATH" <<'PY'
import json
import os
import sys

vault_path = sys.argv[1]


def split_env(name):
    raw = os.environ.get(name, "")
    if not raw:
        return []
    return [p for p in raw.split("\x1f") if p]

out = {
    "vault_path": vault_path,
    "created_dirs": split_env("_VAULT_CREATED_DIRS"),
    "existing_dirs": split_env("_VAULT_EXISTING_DIRS"),
    "created_files": split_env("_VAULT_CREATED_FILES"),
    "existing_files": split_env("_VAULT_EXISTING_FILES"),
    "status": "ok",
}
print(json.dumps(out, indent=2))
PY
