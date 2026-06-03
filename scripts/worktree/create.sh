#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "usage: $0 <kind> <task-id> <slug> [base-branch]" >&2
  exit 1
fi

kind="$1"
task_id="$2"
slug="$3"
base_branch="${4:-dev}"

case "$kind" in
  feat|fix|refactor|docs|spike) ;;
  *)
    echo "invalid kind: $kind" >&2
    exit 1
    ;;
esac

repo_root="$(git rev-parse --show-toplevel)"
common_git_dir="$(git rev-parse --git-common-dir)"
case "$common_git_dir" in
  /*) ;;
  *) common_git_dir="${repo_root}/${common_git_dir}" ;;
esac
worktree_root="${WORKTREE_ROOT:-${repo_root}/.worktrees}"
branch="${kind}/${task_id}-${slug}"
worktree_parent="${worktree_root}/${kind}"
worktree_path="${worktree_parent}/${task_id}-${slug}"

mkdir -p "$worktree_parent"

git fetch origin "$base_branch" --quiet || true

if git show-ref --verify --quiet "refs/heads/${branch}"; then
  git worktree add "$worktree_path" "$branch"
else
  git worktree add -b "$branch" "$worktree_path" "origin/${base_branch}"
fi

cat <<INFO
Created worktree.
- branch: $branch
- path: $worktree_path
INFO
