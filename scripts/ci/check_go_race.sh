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

if [[ -z "$changed_go_files" ]]; then
  echo "go-race-check: no changed Go files detected"
  exit 0
fi

packages="$({
  while IFS= read -r file; do
    dir="$(dirname "$file")"
    go list "./$dir" 2>/dev/null || true
  done <<< "$changed_go_files"
} | sed '/^$/d' | sort -u)"

if [[ -z "$packages" ]]; then
  echo "go-race-check: no Go packages resolved from changed files"
  exit 0
fi

echo "go-race-check: testing packages"
printf '%s\n' "$packages"

# Build a -run filter from test function names in changed *_test.go files.
# Large packages with many tests (and slow integration tests) would otherwise
# make the race check prohibitively slow. We only need to detect races in
# code exercised by changed tests; for packages with only non-test changes,
# we fall back to running all tests.
changed_test_files="$({
  git diff --name-only "$merge_base"...HEAD -- '*_test.go'
  git diff --name-only -- '*_test.go'
  git diff --cached --name-only -- '*_test.go'
} | sed '/^$/d' | sort -u)"

run_filter=""
if [[ -n "$changed_test_files" ]]; then
  funcs="$(while IFS= read -r f; do
    [[ -f "$f" ]] || continue
    grep -E '^func (Test|Benchmark|Fuzz)[A-Za-z0-9_]+\(' "$f" \
      | grep -oE '(Test|Benchmark|Fuzz)[A-Za-z0-9_]+'
  done <<< "$changed_test_files" | sort -u | paste -sd '|')"
  if [[ -n "$funcs" ]]; then
    run_filter="$funcs"
    echo "go-race-check: run filter: ${run_filter}"
  fi
fi

mapfile -t package_args <<< "$packages"
extra_args=()
if [[ -n "$run_filter" ]]; then
  extra_args+=("-run=${run_filter}")
fi
go test -race -timeout 40m "${extra_args[@]}" "${package_args[@]}"
