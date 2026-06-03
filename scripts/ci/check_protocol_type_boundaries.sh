#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

base_ref="${1:-origin/dev}"
if ! git rev-parse --verify "$base_ref" >/dev/null 2>&1; then
  if git rev-parse --verify HEAD~1 >/dev/null 2>&1; then
    base_ref="HEAD~1"
  else
    base_ref="HEAD"
  fi
fi

merge_base="$(git merge-base HEAD "$base_ref" 2>/dev/null || printf '%s' "$base_ref")"

is_domain_type() {
  case "$1" in
    Activity|AgentMessage|ArtifactNode|ArtifactRef|AttachmentRef|Event|EventRecord|Item|Manifest|ProjectConfig|ProjectContext|ProjectState|Run|Session|SoulDoc|Task|Thread|Turn)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

changed_protocol_files="$({
  git diff --name-only "$merge_base"...HEAD
  git diff --name-only --cached
  git diff --name-only
  git ls-files --others --exclude-standard
} | sed '/^$/d' | sort -u | grep '^pkg/protocol/.*\.go$' || true)"

if [[ -z "$changed_protocol_files" ]]; then
  exit 0
fi

added_type_lines_for_file() {
  local file="$1"

  {
    git diff -U0 "$merge_base"...HEAD -- "$file" 2>/dev/null || true
    git diff -U0 --cached -- "$file" 2>/dev/null || true
    git diff -U0 -- "$file" 2>/dev/null || true
  } | awk '
    /^\+\+\+/ { next }
    /^\+type [A-Za-z_][A-Za-z0-9_]* =/ { next }
    /^\+type [A-Za-z_][A-Za-z0-9_]*/ { print substr($0, 2) }
  '

  if git ls-files --others --exclude-standard -- "$file" | grep -q .; then
    awk '
      /^type [A-Za-z_][A-Za-z0-9_]* =/ { next }
      /^type [A-Za-z_][A-Za-z0-9_]*/ { print }
    ' "$file"
  fi
}

violations="$(
  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      type_name="$(awk '{ print $2 }' <<<"$line")"
      if is_domain_type "$type_name"; then
        printf '%s:%s\n' "$file" "$type_name"
      fi
    done < <(added_type_lines_for_file "$file")
  done <<< "$changed_protocol_files"
)"

if [[ -n "$violations" ]]; then
  echo "guardrails: new canonical domain types do not belong in pkg/protocol." >&2
  echo "Move these definitions to pkg/types and reference them from protocol transport structs instead:" >&2
  printf '%s\n' "$violations" | sort -u >&2
  exit 1
fi
