Hunt for meaningful opportunities to improve agen8. This is not a wishlist generator — it is a systematic audit of the codebase, architecture, UI/UX, and product surface to find high-leverage changes that move agen8 toward its vision: agents as spaces that can work with an operator or independently.

## The vision filter

Every finding must pass this test: does it meaningfully advance one of these goals?

1. **Operator-agent parity** — if an operator can do it, agents should be able to do it themselves. If agents can do it, operators should have visibility and control.
2. **Self-improvement** — can agen8 use itself to get better? Features that let agents inspect, refactor, test, and evolve their own platform.
3. **Cohesion** — reducing the gap between concepts that should be unified. Eliminating seams the user shouldn't have to think about.
4. **Complexity reduction** — achieving the same (or better) behavior with less machinery. Fewer states, fewer code paths, fewer concepts to learn.
5. **Workhorse generality** — features that make agen8 useful across professions, not just software engineering. Consulting, research, operations, creative work.

If a finding doesn't serve at least one of these, discard it. Features for the sake of features are noise.

## Anti-sprawl principle (CRITICAL)

**Do not propose one tool per capability gap.** agen8 has ~33 agent tools and ~162 RPC methods. Closing every parity gap with a dedicated tool would double the tool catalog. Tool sprawl burns LLM context window, increases selection latency, and fragments the agent's mental model.

Instead:
- **Follow the multi-command pattern.** `heartbeat_manage` (`pkg/tools/builtins/heartbeat_manage_impl.go`) and `obsidian` (`pkg/tools/builtins/obsidian_impl.go`) use an `action`/`command` parameter to handle multiple operations in one tool. All new capability tools must follow this pattern.
- **Group by intent, not by database table.** Good groupings: `knowledge_graph` (decisions + context links + missions), `inspect` (metrics + logs + events), `admin` (roles + config + notifications). Bad groupings: one tool per RPC domain.
- **Prefer read-only over read-write.** Many parity gaps are intentional guardrails. Agents should be able to *see* metrics, *query* decisions, *traverse* relationships — but not necessarily *modify* space config, *manage* MCP servers, or *configure* notification rules. Those are operator-only for a reason.
- **Ask before proposing write access.** If a finding suggests giving agents write access to platform internals (config, roles, tool sources, policies), flag it explicitly and justify why the operator shouldn't retain exclusive control.

**Reference:** The first run of this command proposed 15+ individual tools and had to close 10 issues. Don't repeat that mistake.

## Setup

**Stay current with dev before starting:**
```
git fetch origin dev
git log HEAD..origin/dev --oneline
```
If there are new commits, rebase or merge origin/dev into your working branch before auditing. You must audit the latest code, not a stale snapshot. If you are on dev directly, fast-forward: `git pull --ff-only origin dev`.

Read these files first — they define what agen8 is and how it works:
- `CLAUDE.md` — project rules, architecture, workflow
- `AGENTS.md` — agent behavior policies
- `docs/architecture/` — all files in this directory
- Skim the key packages: `pkg/agent/space/`, `pkg/services/`, `pkg/runtime/`, `pkg/tools/`, `internal/app/`
- Skim the web UI: `web/src/pages/`, `web/src/components/`, `web/src/hooks/`

Understand what exists before proposing what should exist.

## Discovery passes (in priority order)

Work through each pass sequentially. For each pass, read the relevant code, think critically, and only record findings that are real and actionable.

### Pass 1: Operator-agent parity gaps

The core promise: anything an operator does, an agent should be able to do autonomously. Anything an agent does, the operator should be able to see and steer.

**Audit the operator surface:**
- Read every RPC method in `internal/app/` and `internal/api/`. For each one that an operator can call from the UI or CLI, check: can an agent invoke the same capability via a tool?
- Read every tool in `pkg/tools/`. For each agent tool, check: does the operator have a corresponding UI surface to do the same thing manually, or to see the results?

**Look for gaps:**
- RPC methods with no corresponding agent tool (operator can, agent can't)
- Agent tools with no corresponding UI surface (agent can, operator can't see)
- Settings or configurations only changeable via config files, not at runtime
- Actions that require restarting the daemon when they should be hot-reloadable

**Classify each gap before proposing a fix:**
- **Read gap** (agent can't *see* something) → high-value, low-risk. Propose adding to a composite read tool.
- **Write gap** (agent can't *change* something) → evaluate whether this is an intentional guardrail. Operator-only domains include: config/manifest changes, MCP server management, integration management, tool policy management, notification rule configuration. Do not propose agent write access to these without explicit justification.
- **Steering gap** (operator can't *redirect* something) → high-value for operator trust.

**Check steering capabilities:**
- Can operators redirect agents mid-task? How? Is it real-time or next-turn?
- Can operators inject context without interrupting flow?
- Can agents request operator input without blocking the entire space?

**When proposing new tools, consolidate aggressively.** Group related capabilities into one composite tool using the `heartbeat_manage` action-parameter pattern. Never propose more than 3 new tools from this pass.

### Pass 2: Self-improvement potential

agen8 should be able to improve itself. This means agents running inside agen8 should have the tools and access patterns to work on the agen8 codebase effectively.

**Audit the tool surface for self-work:**
- Can an agen8 space run its own test suite and interpret results?
- Can agents read their own logs, metrics, and error patterns?
- Can agents modify their own prompts, skills, or role definitions and test the changes?
- Can agents create and manage their own worktrees, branches, PRs?
- Is there a feedback loop where agent performance data flows back into agent configuration?

**Look for meta-capabilities:**
- Introspection: can an agent see its own token usage, error rate, task completion rate?
- Adaptation: can an agent modify its own behavior based on outcomes?
- Evolution: can spaces propose and test changes to their own structure?

### Pass 3: Cohesion and conceptual unification

Find places where the user has to think about implementation details they shouldn't care about. Find concepts that are split across the codebase when they should be unified.

**Audit the domain model:**
- Read `pkg/types/`. Are there types that represent the same concept with slightly different shapes? (e.g., task vs message vs activity all carrying "work to do")
- Are there parallel hierarchies that could be flattened? (e.g., separate stores for things that are logically the same entity)
- Is the task state machine (Pending → Active → Succeeded/Failed) sufficient, or are there ad-hoc states scattered in business logic?

**Audit the service boundaries:**
- Read `pkg/services/`. Do services have clear, non-overlapping responsibilities?
- Are there cross-service calls that suggest a missing abstraction?
- Is there duplicated logic across services that belongs in a shared domain concept?

**Audit the UI:**
- Do pages/views map cleanly to domain concepts, or does the user need to visit multiple pages to understand one thing?
- Is navigation predictable? Can the user always get from "I see this" to "I want to act on this" in one click?
- Are there UI concepts that don't map to anything in the domain model (or vice versa)?

### Pass 4: Complexity reduction

Find machinery that could be removed or simplified without losing capability.

**Audit for over-engineering:**
- Abstractions with only one implementation (interface + single struct = unnecessary indirection)
- Configuration options nobody changes (should be hardcoded or removed)
- Features that are implemented but never surfaced in UI or tools (dead weight)
- State machines with states that are never reached in practice
- Middleware, decorators, or wrappers that add no value

**Audit for simplification opportunities:**
- Can any two-step operations become one-step? (e.g., create-then-configure → create-with-config)
- Are there chains of transformations where data is serialized/deserialized multiple times unnecessarily?
- Can the VFS mount system be simplified? Are all mount points actually used?
- Is the runtime build pipeline doing work that could be lazy-loaded?

**Audit the build/test/deploy surface:**
- How many commands does it take to go from zero to running? Can it be fewer?
- Are there test helpers or fixtures that are more complex than the code they test?
- Is there dead code? Unused exports? Abandoned experiments?

### Pass 5: UI/UX modernization

Use web search to research current UI/UX patterns for agent/AI orchestration platforms, space collaboration tools, and developer dashboards. Avoid AI slop — look for patterns from real, shipped products.

**Research (web search required for each):**
- Search: "best practices agent orchestration dashboard UI 2025 2026" — what patterns are real products using?
- Search: "modern space collaboration UI patterns real-time" — how do Slack, Linear, Notion, etc. handle multi-actor workflows?
- Search: "developer dashboard UX patterns observable systems" — how do Datadog, Grafana, Vercel handle complex system visibility?
- Search: "kanban board UX improvements real-time updates" — what's evolved beyond basic column boards?
- Search: "conversational UI patterns multi-agent systems" — how do products display multi-party AI conversations?

**Audit the current UI against findings:**
- Read every page component in `web/src/pages/`. For each one:
  - Is the information density appropriate? Too sparse = wasted screen. Too dense = cognitive overload.
  - Does it handle all states? (empty, loading, error, single item, many items, real-time updates)
  - Are interactions discoverable? Can a new user figure out what to do without a tutorial?
  - Is there wasted vertical space or unnecessary chrome?

**Look for modernization opportunities:**
- Command palette (does one exist? is it comprehensive?)
- Keyboard shortcuts (are there enough? are they discoverable?)
- Contextual actions (right-click, hover actions, inline editing)
- Real-time updates (polling vs SSE vs WebSocket — what's appropriate?)
- Responsive design (does it work on different screen sizes?)
- Accessibility (keyboard navigation, screen readers, contrast)
- Micro-interactions (transitions, feedback on actions, progress indicators)
- Dark/light/dim theme consistency across all components

### Pass 6: Cross-profession generality

agen8 should be a workhorse for any profession, not just engineering. Audit for engineering-specific assumptions.

**Check for hard-coded engineering assumptions:**
- Are tools like `shell_exec`, `code_exec` the only first-class tools? What about research, writing, data analysis, communication?
- Do space templates assume software development workflows?
- Are role definitions biased toward engineering roles (developer, reviewer, tester) vs. general roles (researcher, analyst, writer, strategist)?
- Is the task model flexible enough for non-engineering work? (e.g., "review this document" vs. "review this PR")

**Look for missing profession-agnostic capabilities:**
- Document creation and editing (not just code files)
- Research workflows (web search → synthesis → report)
- Communication workflows (draft email → review → send)
- Data analysis workflows (ingest → analyze → visualize → report)
- Creative workflows (brainstorm → draft → refine → deliver)

**Check the skill system:**
- Read `pkg/skills/` and the default skills. Are they balanced across professions?
- Can skills be composed? Can a "research" skill feed into a "report" skill?
- Is skill discovery good enough that a non-engineer operator can find what they need?

### Pass 7: Architectural leverage points

Find places where a small structural change would unlock disproportionate capability.

**Audit the event system:**
- Read `pkg/events/`. Is the event model rich enough to build features on top of?
- Could events power features they currently don't? (e.g., event-driven dashboards, alerting, audit trails, replay)
- Are events structured consistently? Can they be queried, filtered, aggregated?

**Audit the extension points:**
- How hard is it to add a new tool? A new agent type? A new store backend?
- Are there plugin points that should exist but don't?
- Is the MCP integration complete enough to replace custom tool implementations?

**Audit the data model for future leverage:**
- Is there a concept of "workflow templates" that could be shared, forked, versioned?
- Can space configurations be exported/imported?
- Is there a concept of "recipes" — reusable patterns that operators can apply?

### Pass 8: Missing feedback loops

Find places where information flows one way but should flow both ways.

**Agent → Operator feedback:**
- Can agents surface confidence levels? ("I'm 80% sure about this approach")
- Can agents request partial reviews? ("Check my work on step 3 before I continue")
- Do agents communicate blockers proactively, or only when the operator checks?

**Operator → Agent feedback:**
- Can operators give thumbs up/down on individual agent actions?
- Is there a way to say "never do this again" or "always do it this way" that persists?
- Can operators adjust agent behavior without editing YAML files?

**Agent → Agent feedback:**
- Can agents learn from each other's mistakes within a run?
- Is there a shared knowledge base that improves over the lifetime of a space?
- Can a reviewer agent's feedback change how a worker agent approaches future tasks?

**System → Everyone feedback:**
- Are there dashboards showing trends over time? (improving? degrading?)
- Can the system detect and surface patterns? ("This type of task always fails on first attempt")
- Is there anomaly detection? ("Token usage spiked 10x on this run")

## IMPORTANT: discovery only — do not implement

This command is for FINDING opportunities, not implementing them. Do not modify production code. Do not create feature branches. Your job is to produce high-quality issues that feed into `/new-feature` or `/epic-workflow`.

## Quality bar for findings

Each finding must be:
1. **Specific** — point to exact files, functions, UI components. No hand-waving.
2. **Justified** — explain WHY this matters using the vision filter above. Which goal does it serve?
3. **Scoped** — what would the change look like? Not a full design, but enough to estimate size (small/medium/large).
4. **Distinct** — not a duplicate of an existing issue. Check open issues via GitHub API.
5. **Anti-sprawl compliant** — if proposing a new tool, it must consolidate multiple capabilities into one composite tool.

Reject findings that are:
- Generic advice that could apply to any project ("add more tests", "improve documentation")
- Cosmetic preferences with no behavioral impact
- Features that only serve edge cases
- Complexity additions disguised as improvements
- One-tool-per-RPC-method proposals (tool sprawl)

## What to do with findings

Group findings by pass. **Before creating issues, consolidate aggressively:**

1. Merge related findings into a single issue where possible. Three small findings in the same subsystem = one medium issue, not three issues.
2. For new agent tools: never propose more than 1 new composite tool per pass. Multiple capabilities go into one tool via the action-parameter pattern.
3. Check for existing issues that already cover the finding. Use the GitHub API or MCP tools to search open issues.
4. Present your consolidated findings to the user before creating issues. Get approval on which findings are worth filing.

**Create issues using the GitHub API** (MCP tools or direct API with the repo PAT):

The issue body must include:
- **Opportunity**: what could be better, with exact file paths and current behavior
- **Vision alignment**: which of the 5 vision goals this serves (quote it)
- **Proposed approach**: 2-3 sentences on how to implement, not a full design
- **Size estimate**: small (< 1 day), medium (1-3 days), large (3+ days)
- **Dependencies**: other issues or features this depends on or enables
- **Affected subsystem**: `runtime`, `storage`, `web`, `protocol`, `cli`, `architecture`, or `product`
- **Anti-sprawl check**: if proposing a new tool, explain why it can't be folded into an existing composite tool
- **UI/UX notes**: if applicable, reference the specific modern pattern being applied (with source)

For related findings, create an epic issue that links the sub-issues.

Label conventions:
- `enhancement` for new capabilities
- `refactor` for complexity reduction and cohesion improvements
- `ux` for UI/UX modernization
- `architecture` for structural changes
- Add `self-serve` for features that enable agent autonomy
- Add `parity` for operator-agent parity gaps

## Persistence

When you find patterns or insights about the codebase that would help future agents, append them to the `Lessons learned` section of `AGENTS.md`.

## Lessons from previous runs

These patterns were discovered during prior `/discover` executions. Do not re-discover them — build on them.

### Codebase facts
- **162 RPC methods** across `internal/app/rpc_*.go` files. **33 agent tools** (14 builtin + 19 coordinator/context tools).
- The multi-command pattern (`heartbeat_manage`, `obsidian`) uses an `action`/`command` enum parameter with a switch statement. This is the canonical pattern for composite tools.
- `pkg/types/` and `pkg/protocol/` have systematic type duplication (Task, Role, Space, Message). Already tracked in #418.
- Task state machine has 7+ real states (pending, active, blocked, review_pending, succeeded, failed, canceled) vs 4 documented. Tracked in #1249.
- Agent type prompts (`pkg/agenttype/`) are code-generated and immutable at runtime. Skills (`pkg/skills/`) are loaded at startup and read-only via VFS.
- Confidence floats (0.0–1.0) already exist on `ContextLink` in `pkg/types/context_link.go`.
- `ProjectExport`, `ProjectApply`, `ProjectDiff` RPC methods are registered but appear to be stubs without full backend implementation.

### Operator-only domains (intentional guardrails)
These should remain operator-only unless there's a compelling reason to open them:
- Config/manifest changes (`ManifestUpdate*`, `ConfigUpdate*`)
- MCP server management (`MCPServer*`)
- Integration management (`Integration*`)
- Tool policy management (`Toolpolicy*`)
- Notification rule configuration (`NotificationsRules*`)

### Agent capability gaps worth closing (via composite tools)
- Metrics introspection (read-only) — #1233
- Knowledge graph traversal (decisions + context links + missions) — #1247
- Operator preferences (structured, queryable) — #1256

### UI patterns discovered (2025-2026 research)
- Action-first interfaces over chat-only (agents show proposed actions, operator approves/rejects)
- Keyboard shortcut discoverability is table stakes (Linear, Notion, Grafana all show hints)
- Kanban boards need WIP limits, cycle time, and flow metrics inline — not on a separate page
- Grafana best practice: thresholds + anomaly markers on charts, shared cursors for correlation
- Multi-agent UIs should show orchestration transparency (who delegated what, confidence levels)

## Start now

Begin with Pass 1. Work through each pass sequentially. Report findings as you go — do not batch everything to the end. Use web search actively in Pass 5. Present consolidated findings to the user for approval before mass-creating issues.
