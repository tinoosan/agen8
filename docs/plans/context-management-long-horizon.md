# Context Management for Long-Horizon Sessions

**Branch:** `cursor/agent-context-management-e2a3`  
**Status:** Discussion / Design  
**Goal:** Enable agents to perform long-horizon sessions spanning multiple tasks, especially when working autonomously without user intersection, while remembering enough context to avoid wasting tool calls.

---

## 1. Problem Statement

When agents run for extended periods—either processing many tasks in sequence or working autonomously on self-generated work—they face several context-related challenges:

1. **Session span:** Sessions can span multiple tasks (user goals, callbacks, spawned subagent results). The agent needs continuity across task boundaries.
2. **Autonomous operation:** When agents work without user intersection (e.g., daemon mode, background processing), they must rely entirely on persisted context.
3. **Tool call waste:** Without sufficient memory, agents may:
   - Re-read files they already read
   - Re-run commands they already ran
   - Re-search `/memory` for facts they already know
   - Re-discover project structure or patterns

This wastes tokens, increases latency, and degrades the quality of autonomous runs.

---

## 2. Current State

### 2.1 Context Sources (Today)

| Source | Purpose | Budget / Behavior |
|--------|---------|-------------------|
| `run.maxBytesForContext` | Overall context byte limit | CLI `--context-bytes` (default 8KB) |
| `/memory` | Long-term memory (daily files) | `--memory-bytes` (default 8KB); tool-based `fs_search` for recall |
| `/log` (trace) | Recent ops, events | `--trace-bytes` (default 8KB); incremental since cursor |
| `/history` | Session history (user/agent pairs) | `--history-pairs` (default 8); injected on resume |
| System prompt | Base instructions + memory excerpt | Fixed; memory excerpt removed for token optimization |

### 2.2 Key Components

- **PromptUpdater** (`pkg/agent/context_updater.go`): Refreshes context once per model step; injects trace summary and last host op into system prompt.
- **PromptBuilder** (`pkg/agent/context_constructor.go`): Minimal prompt for autonomous runs; base + today's memory + skill scripts. Intentionally avoids session history and trace.
- **Session** (`pkg/types/session.go`): `CurrentGoal`, `Summary`, `HistoryCursor` support resume; `Summary` is a bounded digest to reduce token cost.
- **HistoryStore**: Session-scoped; append-only; supports incremental retrieval via cursor.

### 2.3 Gaps for Long-Horizon

- **No explicit "what I've already done" cache:** The agent has no structured representation of recent tool invocations (reads, searches, shell runs) to avoid redundant work.
- **Trace is ops-focused, not semantic:** Trace summarizes event types (op.request, op.response, etc.) but doesn't capture "I read X and learned Y."
- **Summary is manual/host-maintained:** Session `Summary` is updated by the host; the agent doesn't write a compact recap of its own progress.
- **History pairs are raw:** Recent history pairs are injected as-is; no summarization or relevance filtering for the current task.
- **Subagent context is isolated:** Subagents see only parent-passed context; parent doesn't automatically get a compact digest of child work beyond the callback.

---

## 3. Discussion Areas

### 3.1 Structured "Recent Work" Cache

**Idea:** Maintain a bounded, structured cache of recent agent actions (reads, searches, shell runs) that the agent can consult before re-invoking tools.

- **Pros:** Reduces redundant reads/searches; agent can reason "I already read X at path Y."
- **Cons:** Adds state; must be pruned/rotated; risk of staleness if files change.
- **Open questions:**
  - Who populates it? (Host observes tool calls vs. agent self-reports)
  - How much to keep? (Last N actions vs. byte budget)
  - How to expose? (System prompt block vs. tool like `recent_work`)

### 3.2 Agent-Written Progress Summaries

**Idea:** Encourage (or require) the agent to write a compact progress summary to a well-known location (e.g., `/workspace/.agent-progress.md` or a session field) at task boundaries or periodically.

- **Pros:** Agent controls what it considers important; human-readable; can be injected on resume.
- **Cons:** Relies on agent discipline; may be inconsistent; adds prompt instructions.

### 3.3 Smarter History Injection

**Idea:** Instead of raw recent history pairs, use summarization or relevance filtering before injection.

- **Pros:** Fewer tokens for same information; can prioritize task-relevant turns.
- **Cons:** Summarization costs tokens; relevance scoring is hard; may lose nuance.

### 3.4 Semantic Trace / Episodic Memory

**Idea:** Extend trace or add a separate "episodic" store that captures higher-level facts: "Read file X; learned that Y uses pattern Z."

- **Pros:** More useful for "what do I already know?" reasoning.
- **Cons:** Requires extraction (agent or host); storage growth; retrieval complexity.

### 3.5 Tool-Level Hints

**Idea:** Tools could return hints like "you already read this file in the last 5 steps" or "fs_search on /memory returned this for similar query."

- **Pros:** Direct feedback at call time; no new context structure.
- **Cons:** Doesn't prevent the call; adds tool complexity; may clutter responses.

### 3.6 Compaction and Pruning

**Idea:** Use LLM compaction (e.g., `CompactConversation`) or rolling summarization to keep conversation history bounded while preserving key facts.

- **Pros:** Fits within existing context window; reduces token cost over time.
- **Cons:** Compaction can lose detail; requires careful prompt design; cost of compaction calls.

### 3.7 Task-Boundary Context Handoff

**Idea:** At task completion (including callback processing), explicitly pass a compact "what happened" digest to the next task's context.

- **Pros:** Clean boundaries; each task starts with relevant prior context.
- **Cons:** Need to define handoff format; coordination tasks may have many callbacks.

---

## 4. Success Criteria (Draft)

- [ ] Agent avoids re-reading the same file within a session when content hasn't changed.
- [ ] Agent avoids re-running identical shell commands when outcome is known.
- [ ] Agent recalls prior `/memory` search results for similar queries.
- [ ] Resume after pause/restart injects enough context that the agent doesn't re-do work.
- [ ] Token cost per task decreases (or stays flat) as session length increases.
- [ ] No regression in single-task or short-session behavior.

---

## 5. Next Steps

1. **Prioritize:** Which of the discussion areas gives the highest leverage for "avoid wasting tool calls"?
2. **Prototype:** Pick one mechanism (e.g., structured recent-work cache) and implement a minimal version.
3. **Measure:** Define metrics (redundant reads, redundant searches, tokens per task) and baseline current behavior.
4. **Iterate:** Refine based on real long-horizon runs.

---

## 6. References

- [pkg-agent-session](architecture/pkg-agent-session.md) – Session lifecycle, callbacks
- [pkg-agent-state](architecture/pkg-agent-state.md) – Task persistence
- [execution-model](execution-model.md) – Subagent lifecycle, coordination
- `pkg/agent/context_updater.go` – PromptUpdater, trace injection
- `pkg/agent/context_constructor.go` – PromptBuilder
- `pkg/types/session.go` – Session fields (Summary, HistoryCursor, CurrentGoal)
