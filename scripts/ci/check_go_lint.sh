#!/usr/bin/env bash
# Run static analysis linters on Go code.
#
# Tools:
#   - staticcheck (honnef.co/go/tools): correctness + simplification analyzers,
#     overlap with the gopls analyzer set
#   - revive (github.com/mgechev/revive): style + stutter + naming
#
# Tool versions are pinned so CI is reproducible -- a new upstream release
# can't introduce a new check that fails an unrelated PR. Bump these
# deliberately in their own commit when we want to adopt new analyzers.
#
# Usage:
#   scripts/ci/check_go_lint.sh                    # run on all packages
#   scripts/ci/check_go_lint.sh ./pkg/userctx/...  # specific path
#   GO_LINT_TOOLS=staticcheck scripts/ci/check_go_lint.sh   # only staticcheck
set -euo pipefail

# Pinned tool versions. Bump in a dedicated PR after running the linter
# locally and clearing any new findings.
STATICCHECK_VERSION="v0.7.0"
REVIVE_VERSION="v1.15.0"

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

tools="${GO_LINT_TOOLS:-staticcheck,revive}"
paths=("$@")
if [[ ${#paths[@]} -eq 0 ]]; then
  if [[ -n "${GO_LINT_PATHS:-}" ]]; then
    # shellcheck disable=SC2206
    paths=(${GO_LINT_PATHS})
  else
    paths=("./...")
  fi
fi

run_staticcheck=0
run_revive=0
IFS=',' read -r -a tool_arr <<< "$tools"
for t in "${tool_arr[@]}"; do
  case "$t" in
    staticcheck) run_staticcheck=1 ;;
    revive) run_revive=1 ;;
    "") ;;
    *) echo "go-lint: unknown tool '$t'" >&2; exit 2 ;;
  esac
done

# ensure_tool installs the pinned version of $name from $pkg if the
# binary on PATH (or in GOPATH/bin) doesn't already report that version.
# $version_arg is the flag (e.g. "-version") that prints a version string
# containing $version (e.g. "v1.15.0").
ensure_tool() {
  local name="$1"
  local pkg="$2"
  local version="$3"
  local version_arg="$4"

  local gobin
  gobin="$(go env GOPATH)/bin"

  local bin=""
  if command -v "$name" >/dev/null 2>&1; then
    bin="$(command -v "$name")"
  elif [[ -x "$gobin/$name" ]]; then
    bin="$gobin/$name"
  fi

  # Match either the bare version ("1.15.0", revive output) or the
  # "v"-prefixed form ("v0.7.0", staticcheck output).
  local vbare="${version#v}"
  if [[ -n "$bin" ]] && "$bin" "$version_arg" 2>&1 | grep -q -F "$vbare"; then
    PATH="$gobin:$PATH"
    export PATH
    return 0
  fi

  echo "go-lint: installing $name@$version from $pkg"
  GOFLAGS=-mod=mod go install "${pkg}@${version}" >&2
  PATH="$gobin:$PATH"
  export PATH
}

if [[ $run_staticcheck -eq 1 ]]; then
  ensure_tool staticcheck honnef.co/go/tools/cmd/staticcheck "$STATICCHECK_VERSION" -version
fi
if [[ $run_revive -eq 1 ]]; then
  ensure_tool revive github.com/mgechev/revive "$REVIVE_VERSION" -version
fi

revive_config="$repo_root/revive.toml"

status=0

if [[ $run_staticcheck -eq 1 ]]; then
  staticcheck_checks="${GO_STATICCHECK_CHECKS:-all,-ST1000,-ST1003,-ST1020,-ST1021,-U1000}"
  echo "go-lint: staticcheck -checks=${staticcheck_checks} ${paths[*]}"
  if ! staticcheck -checks="${staticcheck_checks}" "${paths[@]}"; then
    status=1
  fi
fi

if [[ $run_revive -eq 1 ]]; then
  echo "go-lint: revive ${paths[*]}"
  if ! revive -config "$revive_config" -formatter friendly "${paths[@]}"; then
    status=1
  fi
fi

if [[ $status -ne 0 ]]; then
  echo "go-lint: failed" >&2
fi
exit $status
