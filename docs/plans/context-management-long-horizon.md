# Context Management for Long-Horizon Sessions

**Branch:** `cursor/agent-context-management-e2a3`  
**Status:** Discussion / Design  
**Goal:** Enable agents to perform long-horizon sessions spanning multiple tasks. Keep it simple: treat context as a conversation, persist it, show when it fills up, and notify when compaction runs.

---

## 1. Approach: Conversation-Style Context

**Core idea:** Save context like a conversation. The agent's context is a linear conversation (user, assistant, tool turns) that grows over the session. When it exceeds the budget, we compact it. No need for complex caches, episodic memory, or semantic trace—the conversation *is* the context.

**Why this works:**
- **code_exec** already makes tool calling efficient. The agent can batch multiple operations (reads, writes, shell runs) in a single `code_exec` invocation, so we don't have the downside of looping after each tool call.
- Simpler mental model: context = conversation history.
- Existing infrastructure: we already have compaction (server-side when supported, local fallback), history persistence, and `maxBytesForContext`.

---

## 2. What We Need

### 2.1 Context Window Visibility

Let the user see when the context window is filling up.

- **Metric:** Current context size (bytes or tokens) vs. budget.
- **Where:** TUI header, monitor, or status bar.
- **Behavior:** Show fill level (e.g., "context: 42% full" or "context: 12K / 32K bytes").

### 2.2 Compaction Transparency

Notify the user when compaction runs.

- **Event:** Emit when `compactConversationForBudget` actually compacts (i.e., when we reduce the message list).
- **User-facing:** System message in the feed, e.g. "Context compacted to fit budget (N messages → M)."
- **Rationale:** User should know when prior turns have been summarized/compressed, so they understand why the agent might not recall fine detail.

---

## 3. Current State (Relevant Pieces)

| Component | Role |
|-----------|------|
| `run.maxBytesForContext` | Budget for conversation; triggers compaction when exceeded |
| `compactConversationForBudget` | Server compaction (OpenAI Responses compact) or local summarization fallback |
| `HistoryStore` | Session-scoped append-only history; supports resume |
| `code_exec` | Batches tool calls in one round-trip; avoids per-tool-call loops |

---

## 4. Success Criteria

- [ ] User can see context fill level (current vs. budget).
- [ ] User is notified when compaction runs.
- [ ] Long sessions work without extra complexity; conversation + compaction is sufficient.
- [ ] No regression in single-task or short-session behavior.

---

## 5. References

- `pkg/agent/loop.go` – `compactConversationForBudget`, compaction trigger
- `pkg/llm/openai_client.go` – `CompactConversation` (server-side)
- `pkg/agent/hosttools/code_exec.go` – Batched tool execution
- `pkg/types/run.go` – `MaxBytesForContext`
