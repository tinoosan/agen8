Hunt for bugs in this codebase. This is not a casual review — it is a systematic audit targeting the specific bug classes that have caused repeated production issues.

## Setup

Read these files first. They define what counts as a bug here:
- `CLAUDE.md` — critical rules, zero-tolerance policies
- `AGENTS.md` — no-fallback error policy, test quality rules, backend completion rule

## Bug classes to hunt (in priority order)

This codebase has a recurring pattern: code that silently succeeds when it should fail. Every audit pass below targets a specific variant of this. Do not skim — read the actual code, understand the call chain, and verify the behavior.

### Pass 1: Discarded errors

Search for every instance where an error is ignored:

```
# Bare underscore error discards in production code (exclude _test.go)
grep -rn '_ = ' pkg/ internal/ cmd/ --include='*.go' | grep -v '_test.go' | grep -v 'vendor/'
grep -rn '_ := ' pkg/ internal/ cmd/ --include='*.go' | grep -v '_test.go'
```

For each match:
- Read the function being called. Does it return an error?
- If yes: this is a bug. The error must be handled — returned, logged at an actionable level, or both.
- `_ = logger.Sync()` in a defer is acceptable. Everything else is suspect.

### Pass 2: Nil-guard silent returns

Search for functions that silently return nil/zero when a dependency is nil:

```
grep -rn 'if .* == nil {' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

For each match, check what happens in the nil branch:
- `return nil` or `return ""` or `return 0` when the nil thing is a required dependency → bug. Should return an error.
- `if dep == nil { return nil }` at the start of a method → banned pattern. The caller thinks the operation succeeded.
- `if callback == nil { return nil }` → bug. Callbacks must be required or the nil path must return an error.

### Pass 3: Missing required field validation

Search for JSON unmarshal in tool Execute methods and RPC handlers:

```
grep -rn 'json.Unmarshal' pkg/tools/ internal/app/ --include='*.go' | grep -v '_test.go'
```

For each match:
- Read the struct being unmarshalled into. Which fields are required?
- After unmarshal, is each required field validated (non-empty, non-zero)?
- If validation is missing: the tool/handler will operate on garbage input silently → bug.

### Pass 4: Store operations that silently succeed on missing rows

Search for UPDATE/DELETE operations in the store layer:

```
grep -rn 'UPDATE\|DELETE' internal/store/ --include='*.go' | grep -v '_test.go'
```

For each match:
- Does the code check `RowsAffected()` after the exec?
- If `RowsAffected() == 0`, does it return an error?
- If not: updating a non-existent row silently succeeds → bug. The caller thinks the operation worked.

### Pass 5: Unwired production features

Check that every service, callback, and event publisher used in production is actually wired:

```
# Find all Set*() methods on services
grep -rn 'func (.*) Set' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

For each setter:
- Grep for non-test callers. There MUST be at least one call site in production code (typically `space_daemon.go` or `daemon_supervisor_spawn.go`).
- If only called from `_test.go` files → the feature exists in tests but not in the running app → bug.

```
# Find all Register*() and Wire*() calls
grep -rn 'Register\|Wire' internal/app/ --include='*.go' | grep -v '_test.go'
```

Cross-reference with what the daemon actually starts.

### Pass 6: Test doubles that ignore arguments

Search for mock/stub implementations in test files:

```
grep -rn 'func.*Mock\|func.*Stub\|func.*Fake\|func.*Spy' pkg/ internal/ --include='*_test.go'
```

For each test double:
- Does it use `_` for any domain argument (IDs, filter parameters, tool names, paths)?
- If yes → the test cannot detect regressions when those arguments change → bug in the test.
- Does it return canned values without checking inputs? → It's a mock, not a real implementation. Flag it.

### Pass 7: Loops that swallow per-item errors

```
grep -rn 'for.*range' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

For each loop, check the error handling inside:
- `if err != nil { log.Error(...); continue }` → errors are swallowed, the loop reports success even if every item failed.
- The loop should track error counts, return aggregated errors, or fail on first error depending on context.

### Pass 8: Frontend — unreachable UI states

For frontend files, check:
- Components that fetch data but have no error state rendering
- Components that show loading state but never transition out on error
- Event handlers that call async functions without `.catch()` or try/catch
- State that can become stale (e.g., polling that doesn't handle 404/500)

```
grep -rn 'useEffect\|useMutation\|useQuery\|fetch(' web/src/ --include='*.ts' --include='*.tsx' | grep -v 'node_modules'
```

### Pass 9: Security — injection and trust boundaries

#### 9a: SQL injection
```
grep -rn 'fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE\|fmt.Sprintf.*WHERE' internal/store/ pkg/ --include='*.go' | grep -v '_test.go'
```
Any SQL built with `fmt.Sprintf` using external input is a SQL injection vector. All queries must use parameterized statements (`?` placeholders).

#### 9b: Command injection
```
grep -rn 'exec.Command\|os.StartProcess' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
For each match: is user/agent input passed directly as arguments without validation or sanitization? Can the input contain shell metacharacters, path traversal (`../`), or null bytes?

#### 9c: Path traversal
```
grep -rn 'os.Open\|os.Create\|os.ReadFile\|os.WriteFile\|filepath.Join' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
For each file operation that takes agent or user input as part of the path:
- Is the path validated to stay within an expected directory?
- Can `../` escape the sandbox?
- Is `filepath.Clean` + prefix check applied before any I/O?

#### 9d: XSS in frontend
```
grep -rn 'dangerouslySetInnerHTML\|innerHTML\|v-html' web/src/ --include='*.tsx' --include='*.ts'
```
Any raw HTML injection of agent/user content is an XSS vector. Content from agents, messages, or tool outputs must be rendered as text, not HTML.

#### 9e: Sensitive data exposure
```
grep -rn 'apiKey\|api_key\|secret\|password\|token\|credential' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
For each match: is the value logged, included in error messages, returned in API responses, or written to files without redaction?

### Pass 10: Prompt injection

This is a multi-agent platform where LLM agents execute tools based on instructions. Any path where user content, agent output, or external data flows into a system prompt or tool invocation without sanitization is a prompt injection vector.

#### 10a: Agent-to-agent injection
```
grep -rn 'system.*prompt\|SystemPrompt\|systemMessage\|buildSystemPrompt\|buildPrompt' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
For each prompt construction site:
- Is user/agent content interpolated directly into the system prompt?
- Can an agent's task output or message content influence another agent's instructions?
- Are tool results from one agent passed unescaped into another agent's context?
- Can a worker craft a `final_answer` or task deliverable that modifies the coordinator's next system prompt?

#### 10b: Operator message injection
```
grep -rn 'operator.*message\|user.*message\|MessageContent\|Content.*string' pkg/agent/ internal/app/ --include='*.go' | grep -v '_test.go'
```
For each site where operator/user messages enter the agent loop:
- Can the operator embed instructions that override the system prompt? (e.g., "Ignore previous instructions and...")
- Is the operator message placed in a `user` role message, or is it interpolated into a `system` role where it gains elevated trust?

#### 10c: Tool output injection
```
grep -rn 'ToolResult\|tool.*output\|tool.*result\|ExecuteResult' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
When tool outputs (shell_exec, code_exec, file reads) are fed back to the LLM:
- Is the content sanitized or length-bounded?
- Can a file on disk contain instructions that the agent will follow? (indirect prompt injection via workspace files)
- Can a tool output contain fake tool-call XML/JSON that the LLM parses as a real tool call?

#### 10d: Knowledge graph / context injection
Search for anywhere external data is injected into agent context:
```
grep -rn 'ContextLink\|contextBlock\|<missions>\|<tasks>\|<escalation' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
Can a previously-compromised agent write data to the knowledge graph, missions, or task descriptions that gets injected into another agent's context as trusted instructions?

For this pass, use web search to check for known prompt injection patterns in multi-agent systems. Search for: "multi-agent prompt injection vulnerabilities" and "indirect prompt injection LLM tool use". Apply any relevant findings to this codebase.

### Pass 11: Supply chain and dependency security

#### 11a: Go dependencies
```
go list -m all | wc -l
```
Check the dependency tree:
- Run `go list -m -json all` and check for any modules with `Deprecated` field set.
- Check for known vulnerabilities: `govulncheck ./...` (install with `go install golang.org/x/vuln/cmd/govulncheck@latest` if not available).
- Look for dependencies pulling from non-standard registries or personal GitHub repos that could be hijacked.
- Check `go.sum` is committed and matches `go.mod`.

#### 11b: npm dependencies
```
cd web && npm audit 2>&1
```
- Check for known vulnerabilities in the web dependency tree.
- Look for `postinstall` scripts in dependencies that execute arbitrary code: `grep -r '"postinstall"' web/node_modules/*/package.json | head -20`
- Verify `package-lock.json` is committed and integrity hashes are present.

#### 11c: Build-time code execution
```
grep -rn 'go generate\|go:generate' pkg/ internal/ cmd/ --include='*.go'
```
Any `go:generate` directive runs arbitrary commands at build time. Verify each generator is a known, trusted tool.

```
grep -rn 'scripts/' .github/workflows/ --include='*.yml'
```
Check every script referenced in CI — can any of them be modified by a PR (i.e., are they in the repo, not pinned externally)?

#### 11d: Runtime dependency fetching
```
grep -rn 'http.Get\|http.Post\|net/http\|go-getter\|download' pkg/ internal/ cmd/ --include='*.go' | grep -v '_test.go'
```
Does the daemon fetch code, config, or dependencies at runtime from external URLs? Any runtime fetch is a supply chain vector if the URL or host is compromised.

For this pass, use web search to check for any CVEs on the specific major dependencies. Run `go list -m all` to get the list, then search for known issues on the top 10 by import frequency.

### Pass 12: Concurrency bugs

```
grep -rn 'go func\|go .*(' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

For each goroutine:
- Is there a `sync.WaitGroup`, channel, or context cancellation to prevent goroutine leaks?
- Are shared variables accessed without mutex protection?
- Can the goroutine outlive its parent scope and access freed resources?

Also check:
```
grep -rn '\.Lock()\|\.RLock()' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```
For each mutex: is `defer mu.Unlock()` used consistently? Are there paths where Lock is acquired but Unlock is skipped on early return?

### Pass 13: Resource leaks

```
grep -rn 'os.Open\|sql.Open\|http.Get\|http.Post\|net.Dial\|\.Body' pkg/ internal/ --include='*.go' | grep -v '_test.go'
```

For each resource acquisition:
- Is there a corresponding `defer .Close()`?
- HTTP response bodies: is `resp.Body.Close()` deferred immediately after the error check?
- File handles: is `f.Close()` deferred?
- Database connections: are they returned to the pool?

### Pass 14: Logic bugs — boundary conditions

Read through the domain logic in `pkg/services/` and `internal/app/`. For each service method:
- What happens when a list is empty? Does the function return an error, return empty, or panic on index access?
- What happens with duplicate input (same ID twice)? Is it idempotent or does it create duplicate records?
- What happens at integer boundaries (0, -1, MaxInt)? Are there unchecked arithmetic operations?
- What happens when a string is whitespace-only but non-empty? Do validators catch it?

## IMPORTANT: audit only — do not fix

This command is for FINDING bugs, not fixing them. Do not modify production code. Do not refactor. Do not "quickly fix" something you find. Your job is to produce high-quality issues that another agent can pick up with `/fix-bugs`.

## TDD approach to proving findings

For each bug found, write a failing test that proves it exists:

1. Create a test file (or add to the existing test file for that package).
2. Write a test that exercises the buggy code path with the exact input that triggers the bug.
3. Run the test. It must fail (or demonstrate the wrong behavior — e.g., test proves the error IS silently swallowed).
4. Do NOT fix the production code to make the test pass. The test is evidence, not a fix.

If you cannot write a failing test for a finding, reconsider whether it is a real bug or a hypothetical concern.

## What to do with findings

For each real bug found:
1. Read the surrounding code to understand the full impact — is this a leaf function or does the bug propagate through callers?
2. Write a failing test that proves the bug exists.
3. Check if there's already an open issue for it: `gh issue list --state open --label bug`
4. If no existing issue, create one with your full analysis in the body:
```
gh issue create --label bug --title "fix: <concise description>" --body "..."
```

The issue body must include ALL of the following so the fixing agent has everything it needs:
- **Problem**: what is wrong, with the exact file path and line number
- **Root cause**: why it happens — the code path, the missing check, the wrong assumption
- **Impact**: what breaks — does it silently corrupt data, expose an attack surface, cause a crash, or mislead the user?
- **Reproduction**: the failing test code (paste the full test function). If the test is committed to a branch, reference the branch and commit.
- **Affected files**: every file that will need to change to fix this
- **Acceptance criteria**: concrete checklist — when is this fixed?
- **Affected subsystem**: `runtime`, `storage`, `web`, `protocol`, `cli`, or `docs`

5. Group related findings into a single issue when they share a root cause (e.g., "5 store methods missing RowsAffected checks" is one issue, not five).
6. For security findings, add the `security` label in addition to `bug`.

## What is NOT a bug

- Code style preferences (naming, formatting, comment style)
- Missing features that aren't in any spec
- Performance optimizations without evidence of actual slowness
- "Could be better" refactors with no behavioral impact
- Theoretical vulnerabilities that require already-compromised internal access

If it doesn't silently produce wrong results, silently lose data, or expose a real attack surface, it's not a bug for this audit.

## Persistence

When you find patterns that repeat across the codebase, append them to the `Lessons learned` section of `AGENTS.md` so future agents avoid introducing the same bugs.

## Start now

Begin with Pass 1. Work through each pass sequentially. Report findings as you go — do not batch everything to the end.
