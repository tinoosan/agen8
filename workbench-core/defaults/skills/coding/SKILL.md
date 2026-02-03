---
name: Coding
description: Implement, refactor, debug, and validate code changes safely with best practices.
---

# Instructions

Use this skill for tasks that require writing, improving, or verifying source code. Prefer it when the outcome relies on specific behavior changes, bug fixes, or new automated checks across the codebase.

## When to use

- The user describes a functional requirement, bug, regression, or performance issue.
- A test suite, script, or deployment needs validation.
- New code must be added, refactored, or reviewed for correctness.
- Existing code needs performance optimization or security hardening.

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
- Follow the Boy Scout Rule: leave code cleaner than you found it (within reason).

## Quality checks

- Code passes linting/formatters and existing tests locally.
- Unit/integration tests cover new logic or clearly state limitations.
- Behavior matches acceptance criteria (validated via diff, test, or reasoning).
- No unrelated files modified.
- Changes are reviewable (focused commits, clear diffs).

## Advanced techniques

### Test-Driven Development (TDD)

When requirements are clear, write the test first:

1. **Write a failing test** that captures the expected behavior
2. **Implement the minimum code** to make it pass
3. **Refactor** for clarity and performance
4. **Repeat** for the next behavior

**Benefits**: Forces clear specifications, prevents over-engineering, builds regression safety.

### Refactoring patterns

**Extract function/method**: When code does too much
```go
// Before
func processOrder(o Order) {
    // 50 lines of validation, calculation, db writes
}

// After
func processOrder(o Order) error {
    if err := validateOrder(o); err != nil { return err }
    total := calculateTotal(o)
    return saveOrder(o, total)
}
```

**Replace magic numbers**: Improve readability
```python
# Before
if user.age > 18:

# After
MIN_LEGAL_AGE = 18
if user.age > MIN_LEGAL_AGE:
```

**Guard clauses**: Reduce nesting
```javascript
// Before
function process(data) {
    if (data) {
        if (data.valid) {
            // deep nesting
        }
    }
}

// After
function process(data) {
    if (!data) return;
    if (!data.valid) return;
    // flat logic
}
```

### Debugging strategies

**Binary search**: Comment out half the code to isolate the bug
**Print debugging**: Add strategic log statements to trace execution flow
**Rubber duck**: Explain the problem out loud (or in comments) to spot the issue
**Bisect**: Use `git bisect` to find the commit that introduced the bug
**Minimal reproduction**: Strip away everything unrelated until you have the smallest failing case

### Code review checklist

- [ ] Does this solve the stated problem?
- [ ] Are edge cases handled (null, empty, large inputs)?
- [ ] Is error handling comprehensive?
- [ ] Are variable/function names clear?
- [ ] Is there duplication that should be extracted?
- [ ] Are there security implications (SQL injection, XSS, etc.)?
- [ ] Does this follow the project's conventions?
- [ ] Are commits atomic and well-described?

## Templates

### Bug fix commit message
```
Fix: <brief description>

Problem: <what was broken>
Root cause: <why it broke>
Solution: <how you fixed it>
Testing: <how you verified>
Closes #123
```

### Feature implementation commit
```
Add: <feature name>

What: <what this does>
Why: <business value>
How: <technical approach>
Testing: <test coverage>
```

## Example

**Task**: "Fix the billing summary to include VAT per invoice"

**Execution**:
1. Located invoice rendering logic in `billing/invoice.go`
2. Added `CalculateVAT(subtotal float64) float64` function
3. Updated `Invoice.Total()` to include VAT
4. Modified `TestInvoiceTotal` to verify VAT calculation
5. Ran billing test suite - all pass
6. Summary: VAT now correctly applied to all invoices at 20% rate

## Anti-patterns

- **Big ball of mud**: Don't touch 10 files for a "simple" change - decompose first
- **Premature optimization**: Don't optimize until you measure a real problem
- **Test pollution**: Don't modify shared test fixtures - use factory functions instead
- **Comment debt**: Don't explain bad code with comments - refactor it to be self-explanatory
- **Copy-paste inheritance**: Don't duplicate code - extract common logic

## When NOT to use

- Quick prototypes or proof-of-concepts (optimize for speed, not quality)
- Throwaway scripts (no tests needed)
- Documentation-only changes (use reporting skill instead)

