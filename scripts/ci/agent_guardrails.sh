#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

is_reviewable_path() {
  [[ "$1" != internal/web/dist/* ]]
}

filter_reviewable_paths() {
  while IFS= read -r path; do
    [[ -z "$path" ]] && continue
    if is_reviewable_path "$path"; then
      printf '%s\n' "$path"
    fi
  done
}

has_rg=0
if command -v rg >/dev/null 2>&1; then
  has_rg=1
fi

filter_source_files() {
  if (( has_rg )); then
    rg '\.(go|ts|tsx|js|jsx|py|sh)$'
  else
    grep -E '\.(go|ts|tsx|js|jsx|py|sh)$'
  fi
}

exclude_guardrail_script() {
  if (( has_rg )); then
    rg -v '^scripts/ci/agent_guardrails\.sh$'
  else
    grep -Ev '^scripts/ci/agent_guardrails\.sh$'
  fi
}

matches_docs_update() {
  if (( has_rg )); then
    rg '^docs/(architecture|ai-development-workflow\.md|patterns\.md)'
  else
    grep -E '^docs/(architecture|ai-development-workflow\.md|patterns\.md)'
  fi
}

has_guardrail_exception() {
  local kind="$1"
  local logs
  logs="$(git log --format=%B "$merge_base"..HEAD 2>/dev/null || true)"

  grep -Eiq "^Guardrail-Exception:\s*(all|${kind}|oversized-pr)(\s|$|,)" <<<"$logs"
}

base_ref="${1:-origin/dev}"
if ! git rev-parse --verify "$base_ref" >/dev/null 2>&1; then
  if git rev-parse --verify HEAD~1 >/dev/null 2>&1; then
    base_ref="HEAD~1"
  else
    base_ref="HEAD"
  fi
fi

merge_base="$(git merge-base HEAD "$base_ref" 2>/dev/null || printf '%s' "$base_ref")"
committed_files="$(git diff --name-only "$merge_base"...HEAD)"
working_files="$({
  git diff --name-only
  git diff --cached --name-only
  git ls-files --others --exclude-standard
} | sed '/^$/d' | sort -u)"
changed_files="$(printf '%s\n%s\n' "$committed_files" "$working_files" | sed '/^$/d' | sort -u | filter_reviewable_paths)"

if [[ -z "$changed_files" ]]; then
  echo "guardrails: no changed files detected"
  exit 0
fi

changed_file_count="$(printf '%s\n' "$changed_files" | sed '/^$/d' | wc -l | tr -d ' ')"
committed_line_count="$(git diff --numstat "$merge_base"...HEAD | awk '$3 !~ /^internal\/web\/dist\// { add += $1; del += $2 } END { print add + del + 0 }')"
working_line_count="$({
  git diff --numstat
  git diff --cached --numstat
} | awk '$3 !~ /^internal\/web\/dist\// { add += $1; del += $2 } END { print add + del + 0 }')"
untracked_line_count="$(git ls-files --others --exclude-standard | filter_reviewable_paths | xargs wc -l 2>/dev/null | awk 'END { print $1 + 0 }')"
changed_line_count="$((committed_line_count + working_line_count + untracked_line_count))"

echo "guardrails: base=$base_ref files=$changed_file_count lines=$changed_line_count"

if (( changed_file_count > 25 )); then
  if has_guardrail_exception width; then
    echo "guardrails: allowing width exception from commit trailer"
  else
    echo "guardrails: PR is too wide ($changed_file_count files). Split the change or justify an exception." >&2
    exit 1
  fi
fi

source_files_to_scan="$(printf '%s\n' "$changed_files" | filter_source_files | exclude_guardrail_script || true)"
if [[ -n "$source_files_to_scan" ]]; then
  if printf '%s\n' "$source_files_to_scan" | xargs grep -nE '(TODO|FIXME|WIP|HACK)' -- >/dev/null 2>&1; then
    echo "guardrails: unresolved TODO/FIXME/WIP/HACK markers found in changed files." >&2
    exit 1
  fi
fi

./scripts/ci/check_protocol_type_boundaries.sh "$base_ref"

candidate_pkg_dirs="$(git diff --name-status "$merge_base"...HEAD | awk '$1 ~ /^A/ { print $2 }' | awk -F/ '
  $1 == "pkg" && NF > 2 { print $1 "/" $2 }
  $1 == "internal" && NF > 2 { print $1 "/" $2 }
  $1 == "web" && $2 == "src" && NF > 3 { print $1 "/" $2 "/" $3 }
' | sort -u)"

new_pkg_dirs="$(
  if [[ -n "$candidate_pkg_dirs" ]]; then
    while IFS= read -r dir; do
      [[ -z "$dir" ]] && continue
      if ! git cat-file -e "${merge_base}:${dir}" 2>/dev/null; then
        printf '%s\n' "$dir"
      fi
    done <<< "$candidate_pkg_dirs"
  fi
)"

if [[ -n "$new_pkg_dirs" ]]; then
  if ! printf '%s\n' "$changed_files" | matches_docs_update >/dev/null 2>&1; then
    echo "guardrails: new package or UI areas require accompanying architecture/docs updates." >&2
    printf '%s\n' "$new_pkg_dirs" >&2
    exit 1
  fi
fi

added_basenames="$(git diff --name-status "$merge_base"...HEAD | awk '$1 ~ /^A/ { n = split($2, parts, "/"); print parts[n] }' | sort)"
if [[ -n "$added_basenames" ]]; then
  dup_name="$(printf '%s\n' "$added_basenames" | uniq -d | head -n1)"
  if [[ -n "$dup_name" ]]; then
    if has_guardrail_exception duplicate-filenames; then
      echo "guardrails: allowing duplicate-filenames exception from commit trailer"
    else
      echo "guardrails: duplicate new filenames detected ($dup_name). Consolidate or rename to avoid parallel implementations." >&2
      exit 1
    fi
  fi
fi

echo "guardrails: passed"
