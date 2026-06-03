For each step, the workflow is:

  1. Fetch issue from GitHub — read the spec, TDD tests, acceptance criteria
  2. Search codebase — find prior implementations, adjacent code, patterns to follow
  3. Create worktree — scripts/worktree/create.sh feat <issue#> <slug> from dev
  4. Write TDD tests first — per the issue spec, tests before implementation
  5. Implement — follow existing DDD patterns (domain → infra → app layering)
  [!LOOP START]
  6. Local validation — go test, go vet, go build, check guardrails, local CI checks not covered by the previous commands
  7. Self-review — catch issues before PR (status validation, missing fields, test gaps)
  8. Fix all review issues — no fallbacks, no deferred fixes
  9. Commit + PR — target dev, link issue, include Guardrail-Exception if needed
  10. Review the PR in detail to search for bugs and possible regressions from desired behaviour.
  [!LOOP END condition="no more bugs"]
  11. Monitor CI — ensure green (or confirm failures are pre-existing)
  12. Merge when green then move on to the next issue in the epic and restart from step 1.
