---
name: Coding
description: Implement, refactor, debug, and validate code changes safely.
---

# Instructions

Use this skill for tasks that require writing, improving, or verifying source code. Prefer it when the outcome relies on specific behavior changes, bug fixes, or new automated checks across the codebase.

## When to use

- The user describes a functional requirement, bug, regression, or performance issue.
- A test suite, script, or deployment needs validation.
- New code must be added, refactored, or reviewed for correctness.

## Workflow

1. **Clarify the goal.** Restate the expected behavior, success criteria, and any constraints (e.g., performance, backward compatibility, supported environments).
2. **Locate the relevant area.** Trace to the smallest path that implements the feature/bug—identify files, functions, and inputs/outputs that must change.
3. **Plan the change.** Sketch how the adjustments interact with existing logic. Decide if new tests, mocks, or data fixtures are necessary.
4. **Implement with minimal churn.** Make the change in focused patches, keeping unrelated files untouched. Document assumptions within comments if helpful.
5. **Add or adjust tests.** Ensure new behavior is covered and regressions are prevented. If not feasible (e.g., manual UI steps), note why in the change log or summary.
6. **Validate progressively.** Run the narrowest automated checks first (single unit test), then regional (integration) and finally broad (CI, end-to-end) as required.
7. **Review and summarize.** Explain what changed, why, and mention any remaining manual steps or follow-up work.

## Decision rules

- Prefer updating existing tests before adding new ones when coverage already exists.
- If a change risks breaking other areas, add guards (feature flags, config toggles) or document expected impacts in the summary.
- When in doubt about requirements, surface the ambiguity and propose a safe default.

## Quality checks

- Code passes linting/formatters and existing tests locally.
- Unit/integration tests cover new logic or clearly state limitations.
- Behavior matches acceptance criteria (validated via diff, test, or reasoning).
- No unrelated files modified.

## Example

"Tweaking the billing summary to include VAT per invoice" would involve locating the invoice rendering logic, adding the VAT computation, updating expectations in the summary tests, and running the billing test suite to ensure totals remain accurate.
