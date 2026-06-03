#!/usr/bin/env bash
set -euo pipefail

# Pre-commit hook: auto-format Go files and run fast checks.
# Install: git config core.hooksPath scripts/ci/hooks
# Or:      ln -sf ../../scripts/ci/pre-commit.sh .git/hooks/pre-commit

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

# --- Go formatting (auto-fix) ---
staged_go_files="$(git diff --cached --name-only --diff-filter=ACM -- '*.go' | sed '/^$/d')"
if [[ -n "$staged_go_files" ]]; then
  # Auto-format staged Go files
  printf '%s\n' "$staged_go_files" | xargs gofmt -w
  # Re-stage the formatted files
  printf '%s\n' "$staged_go_files" | xargs git add
  echo "pre-commit: gofmt applied to $(printf '%s\n' "$staged_go_files" | wc -l | tr -d ' ') Go file(s)"
fi

# --- Go vet on changed packages ---
if [[ -n "$staged_go_files" ]]; then
  packages="$({
    while IFS= read -r file; do
      [[ -f "$file" ]] || continue
      dir="$(dirname "$file")"
      go list "./$dir" 2>/dev/null || true
    done <<< "$staged_go_files"
  } | sed '/^$/d' | sort -u)"

  if [[ -n "$packages" ]]; then
    echo "pre-commit: running go vet on changed packages"
    echo "$packages" | xargs go vet
  fi
fi

# --- Guardrails (TODO/FIXME/WIP markers, PR width) ---
echo "pre-commit: checking for TODO/FIXME/WIP/HACK markers in staged files"
staged_source_files="$(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(go|ts|tsx|js|jsx)$' || true)"
if [[ -n "$staged_source_files" ]]; then
  if printf '%s\n' "$staged_source_files" | xargs grep -nE '(TODO|FIXME|WIP|HACK)' -- >/dev/null 2>&1; then
    echo "pre-commit: TODO/FIXME/WIP/HACK markers found in staged files:" >&2
    printf '%s\n' "$staged_source_files" | xargs grep -nE '(TODO|FIXME|WIP|HACK)' -- >&2
    exit 1
  fi
fi

# --- Web lint on changed frontend files ---
staged_web_files="$(git diff --cached --name-only --diff-filter=ACM -- 'web/src/*.ts' 'web/src/*.tsx' 'web/src/*.js' 'web/src/*.jsx' | sed '/^$/d')"
if [[ -n "$staged_web_files" ]]; then
  echo "pre-commit: running web lint"
  (cd web && npm run lint --silent 2>&1) || {
    echo "pre-commit: web lint failed" >&2
    exit 1
  }
fi

# --- Web tests on changed frontend files ---
staged_web_src_files="$(git diff --cached --name-only --diff-filter=ACM | grep -E '^web/src/.*\.(ts|tsx|js|jsx)$' || true)"
if [[ -n "$staged_web_src_files" ]]; then
  echo "pre-commit: running web tests"
  (cd web && npm test --silent -- --run 2>&1) || {
    echo "pre-commit: web tests failed" >&2
    exit 1
  }
fi

echo "pre-commit: passed"
