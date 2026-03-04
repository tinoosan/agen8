# Testing Skill

## Purpose

This skill teaches the agent how to design, write, evaluate, and improve tests across different programming languages and environments.

The goal is to produce tests that:
- validate real system behaviour
- detect regressions
- remain stable during refactoring
- protect system guarantees
- improve system reliability

Tests should act as behavioral specifications of the system, not enforcement of implementation details.

## Core Philosophy

Tests must verify what the system does, not how the system works internally.

Good tests:
- validate observable behaviour
- verify public interfaces
- detect regressions
- remain stable when internal code changes

Bad tests:
- depend on internal architecture
- verify internal function calls
- inspect internal data structures
- break during refactoring

If internal implementation changes but behaviour remains identical, tests should continue to pass.

## Behaviour-Driven Testing

Tests must focus on externally observable behaviour.

Verify:
- returned outputs
- visible state changes
- interactions with external systems
- guarantees provided by the public API

Avoid verifying:
- internal helper functions
- private methods
- internal module interactions
- internal struct/class state
- specific algorithm steps

Tests should read like behavioural documentation.

Preferred structure:
- Arrange -> Act -> Assert
- Given -> When -> Then

## Refactor-Safe Testing

Tests must remain valid when the internal architecture changes.

Before finalizing any test, evaluate:

"If the internal implementation changed but behaviour stayed the same, would this test still pass?"

If the answer is no, rewrite the test.

Refactor-safe tests:
- depend only on public behaviour
- avoid architectural assumptions
- verify system outcomes rather than internal steps

## Mocking Rules

Mocks should represent only external system boundaries.

Allowed mocks:
- network services
- databases
- filesystem
- external APIs
- message queues
- clocks
- randomness

Do not mock:
- internal services
- domain logic
- modules within the same codebase

Prefer real objects and real logic whenever possible.

Excessive mocking creates brittle tests tied to implementation.

## Test Double Construction

When a fake or stub is required to satisfy an interface or dependency, keep it minimal and explicit.

Prefer lightweight test doubles over heavy mock frameworks:
- implement only the methods the test exercises
- use configurable function fields or callbacks to customize behaviour per test case
- avoid recording all calls by default; record only what the test needs to assert

Test doubles should be as simple as possible while faithfully representing the external boundary they replace.

Avoid generating mocks when a handwritten fake is simpler and clearer.

## Test Isolation

Each test must produce consistent results regardless of execution order.

Prefer fully isolated test setup over shared state between tests.

Use temporary directories, in-memory stores, or isolated database schemas per test. Most language testing frameworks provide utilities for scoped teardown (e.g., `t.TempDir()` in Go, `tmp_path` in pytest, `beforeEach`/`afterEach` in Jest).

When grouping subtests under a parent, be explicit about which subtests depend on shared state and document the dependency clearly.

Avoid global or package-level test state that persists between test functions.

## Asynchronous and Concurrent Test Synchronization

Tests for concurrent or asynchronous systems must not use `sleep` as a synchronization mechanism.

Prefer deterministic signals:
- use channels, events, or callbacks to signal that an operation has started or completed
- use framework utilities (e.g., `asyncio.wait_for`, `Promise.race`, `context.WithTimeout`) to bound how long a test waits
- encapsulate any unavoidable polling into a helper function that communicates clearly what it is waiting for and fails with a descriptive message on timeout

Never guess at timing. If a test requires a `sleep` to pass reliably, the design needs a proper synchronization point instead.

Tests for concurrent systems must verify:
- operations complete within a bounded time
- goroutines, threads, or async tasks stop when cancellation is signalled
- resources are cleaned up after shutdown

Run tests with the race detector or equivalent concurrency analysis tool where available.

## Polling Helpers

When polling for a condition is unavoidable, encapsulate it in a named helper that:
- accepts a clear condition and a failure message
- uses a bounded timeout
- is marked as a test helper so failure reports point to the call site, not inside the helper
- uses a short, fixed poll interval rather than exponential backoff (which adds test latency)

Polling helpers that assert negative conditions ("nothing should happen") must run for a fixed observation window, not exit early on first check.

## Cancellation and Shutdown Testing

When a component accepts a cancellation signal (context, CancellationToken, AbortSignal, etc.), tests must verify:
- the component stops promptly after cancellation
- in-progress operations are recorded with an appropriate terminal status
- the component does not leak goroutines, threads, or open file handles after shutdown

Verification pattern:
1. Start the component in a goroutine or background task.
2. Wait until the component is observably active.
3. Signal cancellation.
4. Wait for the component to stop, with a timeout that fails the test if exceeded.
5. Assert any side effects (status records, events) produced during shutdown.

Never use `sleep` to wait for shutdown.

## Idempotency Testing

When a system exposes operations that must be safe to repeat, tests must verify idempotency explicitly.

Pattern:
1. Perform the operation once and verify the result.
2. Perform the same operation a second time.
3. Verify the second call returns the same result as the first, or returns a safe no-op response.
4. Verify system state is unchanged from after the first call.

This applies to:
- completing or cancelling already-terminal records
- creating resources that already exist
- acquiring leases already held
- writes that must be safe to retry

## Typed Error Testing

When a function returns typed or sentinel errors, tests must verify error identity using the appropriate mechanism for the language, not string comparison.

Examples:
- Go: `errors.Is(err, ErrFoo)` or `errors.As(err, &target)`
- Python: `isinstance(exc.value, FooError)` via `pytest.raises`
- JavaScript/TypeScript: `instanceof FooError` or checking `.code` properties
- Java/C#: `assertThrows(FooException.class, ...)` or checking exception type

Test both the error case (the typed error is returned) and the success case (no error or a different error type is returned) to validate the full error contract.

Do not assert on error message strings unless the message itself is part of the public API contract.

## Error Handling Tests

Tests should verify:
- correct error propagation
- invalid inputs are rejected
- meaningful error information is returned
- failure conditions are handled correctly

Avoid asserting exact error strings unless they are part of the public contract.

## Boundary Testing

Test edge conditions such as:
- empty inputs
- invalid configuration
- large inputs
- concurrency limits
- timeout behaviour
- partial failures

Boundary testing helps detect hidden failure modes.

## Integration vs Unit Testing Strategy

Testing should balance multiple layers.

### Behaviour Tests

Focus on public behaviour of modules or services.

### Integration Tests

Validate interaction between components.

### System Tests

Verify full workflows across subsystems.

Prefer testing behaviour through the public interface rather than isolating internal units unnecessarily.

## Deterministic Testing

Tests should produce consistent results.

Avoid nondeterministic behaviour by controlling:
- randomness
- time
- concurrency timing
- external state

Use deterministic inputs whenever possible.

## Property and Fuzz Testing

Where appropriate, tests should verify properties rather than specific cases.

Examples:
- input validation properties
- invariants
- state consistency

Fuzz testing may be used to detect unexpected failures with varied input data.

## Performance Testing

Performance tests should verify:
- latency expectations
- throughput limits
- memory behaviour under load

Performance tests should avoid micro-optimisation assumptions and instead verify system behaviour under realistic workloads.

## Agent-Driven Test-First Development

When modifying code, prefer a test-first workflow.

1. Identify the behaviour that must change.
2. Write a failing test describing the desired behaviour.
3. Implement the minimal change required to pass the test.
4. Run the test suite.
5. Refactor safely while ensuring tests continue to pass.

This workflow reduces regression risk during automated code modification.

## Regression Protection

When modifying existing code:
- ensure previous behaviour remains validated by tests
- update tests only when behaviour intentionally changes
- add regression tests for discovered bugs

Regression tests must reproduce the original failure before validating the fix.

## Language-Specific Guidance

### Go

- Prefer table-driven tests with `t.Run` subtests for grouping related scenarios.
- Test exported functions and methods only; avoid testing private helpers directly.
- Call `t.Parallel()` at the top of test functions that are safe to run concurrently.
- Mark all test helper functions with `t.Helper()` so failure output points to the call site.
- Use `t.TempDir()` for filesystem isolation; the runner cleans up automatically.
- Use `errors.Is` and `errors.As` for error comparison; never compare error message strings unless they are an explicit public contract.
- Use buffered channels and `context.WithTimeout` to synchronize concurrency tests without `time.Sleep`.
- Use function-field fakes for lightweight interface implementations where each test can inject its own behaviour.
- Prefer `t.Fatalf` for setup errors; prefer `t.Errorf` for assertion failures that allow the test to continue producing useful output.
- Run tests with `-race` to detect data races.

### Python

- Prefer pytest-style tests over unittest subclasses.
- Use fixtures (`@pytest.fixture`) for setup and teardown, scoped appropriately (`function`, `module`, `session`).
- Use `@pytest.mark.parametrize` for table-driven scenarios.
- Use `pytest.raises` with `match` or type checks for exception assertions; avoid catching exceptions manually.
- Use `tmp_path` or `tmp_path_factory` fixtures for filesystem isolation.
- Avoid patching internal functions; patch only at system boundaries (`unittest.mock.patch` on external clients).
- Use `asyncio.wait_for` or pytest-asyncio timeouts to bound async tests; never use `asyncio.sleep` to wait for side effects.
- Use `freezegun` or dependency injection to control time in tests.
- Test behaviour through public module interfaces, not internal implementation details.

### JavaScript / TypeScript

- Test exported module APIs; never import private or internal modules in tests.
- Prefer behavioural assertions over interaction assertions (avoid `expect(mock).toHaveBeenCalledWith`).
- Use `jest.useFakeTimers()` or equivalent only to control time-dependent behaviour; restore real timers after the test.
- Keep async tests explicit: `async/await` for all async operations, with `expect.assertions(n)` where the count matters.
- Use `beforeEach`/`afterEach` for scoped setup and teardown; avoid shared mutable state between tests.
- Prefer in-memory fakes over module-level mocks; use `jest.mock` only at external boundaries (HTTP clients, file system).
- Use `Promise.race` with a rejection timeout to bound async test waiting rather than `setTimeout` with arbitrary durations.
- Verify error types using `instanceof` or error code checks; avoid asserting on error message strings.

### Java / C#

- Test public classes and service interfaces; inject dependencies through constructors to allow substitution in tests.
- Use `@BeforeEach`/`@AfterEach` (JUnit 5) or `[SetUp]`/`[TearDown]` (NUnit) for test lifecycle management.
- Parameterize related scenarios using `@ParameterizedTest`/`@ValueSource` (JUnit 5) or `[TestCase]` (NUnit).
- Prefer stub or fake implementations over `Mockito.verify` / `Mock.Verify` for interaction assertions; only verify interactions at true external boundaries.
- Assert exception types using `assertThrows` (JUnit 5) or `Assert.Throws` (NUnit); avoid try/catch patterns in tests.
- Use `@TempDir` (JUnit 5) or `TestContext.DeploymentDirectory` for filesystem isolation; clean up in teardown.
- For async code, use `CompletableFuture.get(timeout, unit)` or `await Task.WhenAny(task, Task.Delay(timeout))` to bound async assertions.
- Verify contract-level behaviour, not framework wiring details (e.g., do not assert DI container registrations).

### Rust

- Test public APIs and module behaviour through `pub` interfaces.
- Place unit tests in a `#[cfg(test)]` module within the same file; place integration tests in the `tests/` directory.
- Prefer integration tests when validating system behaviour across module boundaries.
- Use `tempfile::TempDir` or `std::env::temp_dir()` for filesystem isolation; drop the `TempDir` to trigger cleanup.
- Use `proptest` or `quickcheck` for property-based testing of invariants.
- For async code, use `tokio::time::timeout` or `async_std::future::timeout` to bound async test waits; avoid `tokio::time::sleep` as a synchronization mechanism.
- Use `tokio::sync::oneshot` or `tokio::sync::Notify` for goroutine-equivalent signaling in async tests.
- Avoid coupling tests to internal implementation; test behaviour through public traits and types.

### Other Ecosystems

- Apply the same principles: test observable behaviour through stable public contracts.
- Use ecosystem-native runners, fixtures, and parameterization patterns.
- Prefer deterministic control over time, randomness, and IO boundaries.
- Use the ecosystem's standard mechanisms for scoped teardown and temporary resources.

## Writing Good Tests

A good test clearly answers:

"What behaviour does the system guarantee?"

Example test names:
- should_execute_task_successfully
- should_retry_failed_job
- should_reject_invalid_configuration
- should_timeout_long_running_operation
- TestContextCanceled_RecordsTaskCanceled (Go style)
- test_claim_is_idempotent_when_already_terminal (Python style)

Avoid names that reference internal mechanics.

## Test Readability

Tests should communicate clearly:
- the scenario
- the action performed
- the expected behaviour

Avoid overly complex test logic.

Tests should function as living documentation of system behaviour.

## Continuous Test Improvement

When reviewing or generating tests:
- detect brittle tests
- remove unnecessary mocks
- simplify complex tests
- increase behavioural coverage
- maintain refactor safety
- improve clarity and reliability

Testing must evolve alongside the system while maintaining confidence in behaviour.

## Validation of External System Interactions

When behaviour depends on external systems, verify contract-level guarantees:
- request and response schema compatibility
- authentication and authorization behaviour
- idempotency and retry safety
- timeout and backoff handling
- error translation and fallback behaviour
- data persistence and retrieval integrity

Test strategy for external interactions:
- keep fast unit tests with boundary fakes for deterministic failure paths
- add integration tests against representative environments or containers
- add contract tests to detect API drift
- record and assert critical side effects that users rely on

Avoid asserting provider internals; assert only the guarantees your system exposes.

## Goal

Tests generated or modified by the agent must:
- validate behaviour
- remain stable during refactoring
- avoid coupling to implementation details
- improve system reliability
- remain readable and maintainable

The purpose of testing is confidence in system behaviour, not enforcement of internal architecture.
