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
changed_go_files="$({
  git diff --name-only "$merge_base"...HEAD -- '*.go'
  git diff --name-only -- '*.go'
  git diff --cached --name-only -- '*.go'
} | sed '/^$/d' | sort -u)"

# Filter out deleted files that no longer exist on disk.
existing_go_files=""
while IFS= read -r f; do
  [[ -f "$f" ]] && existing_go_files="${existing_go_files:+$existing_go_files
}$f"
done <<< "$changed_go_files"
changed_go_files="$existing_go_files"

if [[ -z "$changed_go_files" ]]; then
  echo "gofmt-check: no changed Go files detected"
  exit 0
fi

if [[ -n "$(printf '%s\n' "$changed_go_files" | xargs gofmt -l)" ]]; then
  echo "gofmt-check: formatting drift detected in changed Go files:" >&2
  printf '%s\n' "$changed_go_files" | xargs gofmt -l >&2
  exit 1
fi

echo "gofmt-check: passed"
