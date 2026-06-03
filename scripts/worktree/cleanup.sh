#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <branch>" >&2
  exit 1
fi

branch="$1"
repo_root="$(git rev-parse --show-toplevel)"
worktree_path="$(git worktree list --porcelain | awk -v target="$branch" '
  $1 == "worktree" { path=$2 }
  $1 == "branch" && $2 == "refs/heads/" target { print path }
')"

if [[ -n "$worktree_path" && -d "$worktree_path" ]]; then
  git worktree remove "$worktree_path"
fi

if git show-ref --verify --quiet "refs/heads/${branch}"; then
  git branch -D "$branch"
fi

printf 'Cleaned branch %s\n' "$branch"
printf 'Repository root: %s\n' "$repo_root"
