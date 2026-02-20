#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: vault_search.sh [--tag <tag>] [--link <target>] [--type <F|L|P|MOC|JOURNAL>] [--query <text>] [--in <glob-or-subdir>] [--path <dir>]

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

TAG=""
LINK=""
TYPE=""
QUERY=""
IN_SCOPE=""
VAULT_PATH=""

while [ $# -gt 0 ]; do
  case "$1" in
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --link)
      LINK="${2:-}"
      shift 2
      ;;
    --type)
      TYPE="${2:-}"
      shift 2
      ;;
    --query)
      QUERY="${2:-}"
      shift 2
      ;;
    --in)
      IN_SCOPE="${2:-}"
      shift 2
      ;;
    --path)
      VAULT_PATH="${2:-}"
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
if [ ! -d "$VAULT_PATH" ]; then
  echo "[]"
  exit 0
fi

python3 - "$VAULT_PATH" "$TAG" "$LINK" "$TYPE" "$QUERY" "$IN_SCOPE" <<'PY'
import fnmatch
import json
import os
import re
import sys
from pathlib import Path

vault = Path(sys.argv[1]).expanduser().resolve()
tag_filter = sys.argv[2].strip()
link_filter = sys.argv[3].strip()
type_filter = sys.argv[4].strip().upper()
query_filter = sys.argv[5]
in_scope = sys.argv[6].strip()

WIKILINK_RE = re.compile(r"\[\[([^\]]+)\]\]")
INLINE_TAG_RE = re.compile(r"(^|\s)#([\w\-/]+)")


def parse_frontmatter(text: str):
    if not text.startswith("---\n"):
        return "", text
    end = text.find("\n---\n", 4)
    if end == -1:
        return "", text
    front = text[4:end]
    body = text[end + 5 :]
    return front, body


def parse_type(front: str) -> str:
    m = re.search(r"(?m)^type:\s*['\"]?([^'\"\n]+)", front)
    return m.group(1).strip().upper() if m else ""


def parse_title(front: str, fallback: str) -> str:
    m = re.search(r"(?m)^title:\s*['\"]?([^'\"\n]+)", front)
    return m.group(1).strip() if m else fallback


def parse_tags(front: str):
    tags = set()
    m = re.search(r"(?m)^tags:\s*\[([^\]]*)\]", front)
    if m:
        for t in m.group(1).split(","):
            t = t.strip().strip("'\"")
            if t:
                tags.add(t)
    lines = front.splitlines()
    in_tags_block = False
    for line in lines:
        if re.match(r"^tags:\s*$", line.strip()):
            in_tags_block = True
            continue
        if in_tags_block:
            m2 = re.match(r"^\s*-\s*(.+)$", line)
            if m2:
                t = m2.group(1).strip().strip("'\"")
                if t:
                    tags.add(t)
                continue
            if line.strip() == "":
                continue
            in_tags_block = False
    return tags


def normalize_link(token: str) -> str:
    tok = token.strip().strip("[]")
    if "|" in tok:
        tok = tok.split("|", 1)[0]
    if "#" in tok:
        tok = tok.split("#", 1)[0]
    tok = tok.strip().rstrip("/")
    if tok.lower().endswith(".md"):
        tok = tok[:-3]
    tok = os.path.basename(tok)
    return tok.lower()


def in_scope_match(rel: str) -> bool:
    if not in_scope:
        return True
    rel_norm = rel.replace("\\", "/")
    scope = in_scope.strip().replace("\\", "/").strip("/")
    if any(ch in scope for ch in "*?[]"):
        return fnmatch.fnmatch(rel_norm, scope)
    return rel_norm.startswith(scope + "/") or rel_norm == scope


results = []

for root, _, files in os.walk(vault):
    for name in files:
        if not name.lower().endswith(".md"):
            continue
        path = Path(root) / name
        rel = path.relative_to(vault).as_posix()
        if not in_scope_match(rel):
            continue

        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            text = path.read_text(encoding="utf-8", errors="replace")

        front, body = parse_frontmatter(text)
        note_type = parse_type(front)
        title = parse_title(front, path.stem)
        front_tags = parse_tags(front)
        inline_tags = {m.group(2) for m in INLINE_TAG_RE.finditer(body)}
        all_tags = front_tags | inline_tags

        links = []
        normalized_links = set()
        for m in WIKILINK_RE.finditer(text):
            raw = m.group(1).strip()
            links.append(raw)
            normalized_links.add(normalize_link(raw))

        matched = []
        snippets = []

        if type_filter:
            if note_type == type_filter:
                matched.append("type")
            else:
                continue

        if tag_filter:
            if tag_filter in all_tags:
                matched.append("tag")
                snippets.append(f"tag:{tag_filter}")
            else:
                continue

        if link_filter:
            nlf = normalize_link(link_filter)
            if nlf in normalized_links:
                matched.append("link")
                snippets.append(f"link:{link_filter}")
            else:
                continue

        if query_filter:
            q = query_filter.lower()
            hit_lines = [ln.strip() for ln in text.splitlines() if q in ln.lower()]
            if hit_lines:
                matched.append("query")
                snippets.extend(hit_lines[:3])
            else:
                continue

        if not any([tag_filter, link_filter, type_filter, query_filter]):
            matched.append("all")

        results.append(
            {
                "file": str(path),
                "relative": rel,
                "title": title,
                "types": [note_type] if note_type else [],
                "matched": matched,
                "snippets": snippets,
            }
        )

results.sort(key=lambda x: x["relative"])
print(json.dumps(results, indent=2))
PY
