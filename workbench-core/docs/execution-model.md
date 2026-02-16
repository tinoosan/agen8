# Sub-Agent & Team Execution Model (Protocol v2)

**Status:** Draft  
**Scope:** Execution hierarchy, authority rules, retry/escalation behaviour, lifecycle management, and daemon responsibilities for standalone and team modes.  
**Non-Goal:** Database schema, struct definitions, or field-level design; implementation details are inferred from the existing architecture.

This document is the **authoritative product specification** for how agents, sub-agents, and teams execute. Implementation (daemon, session, TUI) must conform to it. See [Subagent verification](subagent-verification.md) for how to validate behaviour and [Developer guide](developer-guide.md) for architecture context.

---

## 1. Objective

Establish a clear, enforceable execution model that:

- Supports standalone agents and team-based agents
- Allows sub-agents for isolated execution work
- Prevents architectural drift into uncontrolled swarm behaviour
- Enforces predictable retry and escalation paths
- Keeps coordination logic separate from execution logic
- Moves shared functionality into the daemon RPC layer so future Web UI clients can rely on stable semantics

The system must prioritise **clarity**, **containment**, and **enforceable authority boundaries**.

---

## 2. Core Principles

### 2.1 Tree-Based Authority Model

The execution structure must always form a **strict hierarchy**.

- **Standalone mode:** one root agent
- **Team mode:** one coordinator and multiple co-agents
- **Sub-agents are always leaves**
- No lateral communication between sibling agents
- No graph or mesh topology

The runtime must enforce this.

### 2.2 Single Parent Ownership

Every sub-agent must have **exactly one parent** agent.

- Ownership cannot change
- All reporting must flow upward to the parent
- Sub-agents must never communicate directly with coordinators or other agents

### 2.3 Separation of Roles

**Coordinator** responsibilities:

- Delegation
- Review
- Escalation resolution
- Strategic planning

**Co-agent** responsibilities:

- Execution of assigned work
- Optional spawning of sub-agents
- Reviewing sub-agent outputs

**Sub-agent** responsibilities:

- Execute a narrowly scoped task
- Retry if instructed
- Report results

**The coordinator must not be allowed to spawn sub-agents.** This is a hard rule.

---

## 3. Modes of Operation

### 3.1 Standalone Mode

- A single root agent exists
- The root agent may spawn sub-agents
- Escalation resolves to user-level interaction
- There is no coordinator

Sub-agents are optional but allowed.

### 3.2 Team Mode

- Exactly one coordinator
- One or more co-agents
- Coordinator delegates tasks to co-agents
- Co-agents may spawn sub-agents
- **Coordinator cannot spawn sub-agents**

**Escalation path:** Sub-agent → Parent Co-Agent → Coordinator → Resolution  
No other path is permitted.

---

## 4. Sub-Agent Lifecycle

### 4.1 Creation

A sub-agent is created only when:

- A parent agent explicitly requests it
- The task is owned by that parent

The system must:

- Bind the sub-agent to the parent
- Ensure it is scoped to the parent task lifecycle

Sub-agents are **not** long-lived team members; they are **execution workers**.

### 4.2 Execution

Sub-agents:

- May execute tools
- May produce artifacts
- May produce summaries
- **Must not** assign tasks
- **Must not** spawn additional agents
- **Must not** communicate laterally

All outputs must return to the parent.

### 4.3 Review Gate

Sub-agent work is **not** considered complete until the parent agent explicitly approves it.

Completion requires:

- A structured summary
- Artifacts if produced
- Status marking as needing review

The parent must respond with one of:

- **Approve**
- **Retry**
- **Escalate** (team mode only)

---

## 5. Retry Policy (Reuse Same Sub-Agent)

Retries must **reuse the same sub-agent instance**.

**Rationale:** Preserves context, reduces token waste, simplifies lifecycle management.

**Requirements:**

- Each sub-agent must have a defined retry budget
- Retries must not exceed the configured maximum
- Each retry must be recorded as a distinct attempt for auditability
- The parent must provide a reason for retry
- The sub-agent must incorporate feedback into the next attempt

**If retry budget is exhausted:**

- In **standalone mode** → escalate to user
- In **team mode** → escalate to coordinator

---

## 6. Escalation Model

Escalation may **only** be initiated by the **parent** agent. Sub-agents cannot escalate directly.

**In team mode:**

- Escalation creates a coordinator-visible review unit
- The coordinator may: Approve, Reassign to another co-agent, Request further work

**In standalone mode:**

- Escalation surfaces as user-facing intervention

Escalation must always include:

- Attempt summary
- Parent recommendation
- Relevant artifacts

---

## 7. Cleanup Behaviour

Sub-agents must be **explicitly cleaned up** after:

- Approval
- Final failure
- Escalation resolution

**Cleanup must:**

- Remove execution context (stop worker, release resources)
- Preserve indexed artifacts
- Preserve audit history (run records remain in the store; only the live worker is removed)

**Cleanup must not occur while:**

- Running
- Awaiting retry

---

## 8. Inbox / Task Routing Requirements

The system must:

- Maintain a unified task routing mechanism
- Allow both standalone and team modes to use the same underlying task pipeline
- Route tasks based on explicit ownership

Private inboxes are an implementation detail; they must not define system semantics.

All routing, assignment, and status transitions must be governed by **daemon-level logic**.

---

## 9. Shared Workspace & Attribution

Artifacts must:

- Be indexed at team/session level
- Preserve attribution of producing agent
- Attribute deliverables to parent role for UX clarity

Sub-agent artifacts must **not** create fragmentation in the browsing experience. Sub-agent outputs live under the **parent’s workspace** (e.g. `workspace/subagents/<childRunId>/`) so the parent can review them and the main agent sees a single workspace tree.

Sub-agents are execution workers; they are **not** first-class team roles.

---

## 10. Daemon RPC Layer Responsibilities

All behaviour shared by multiple clients (TUI, future Web UI, external API) must be implemented in the **daemon layer**.

This includes:

- Task creation
- Sub-agent spawning
- Retry enforcement
- Escalation handling
- Review gating
- Cleanup rules
- State transitions
- Artifact indexing
- Hierarchy validation

**The UI must not contain business logic.** Clients must call RPC methods that enforce these invariants.

Future web interfaces must be able to rely on:

- Deterministic state transitions
- Centralised authority enforcement
- Stable execution contracts

No execution semantics should exist exclusively inside the TUI.

---

## 11. Hard Constraints (Runtime Enforced)

The daemon must **reject** operations that violate the following:

- Coordinator attempting to spawn sub-agents
- Sub-agent attempting to assign tasks
- Sub-agent attempting lateral communication
- Retry exceeding configured maximum
- Task completion without parent approval (review gate)
- Escalation initiated by sub-agent
- Multiple parents for a sub-agent

Violations must produce **explicit runtime errors**.

---

## 12. Guardrails Against Swarm Drift

The system must **not** evolve into:

- Peer-to-peer agent networks
- Voting-based coordination
- Unbounded sub-agent recursion

All agent hierarchies must remain **strictly controlled and tree-based**.

---

## 13. Success Criteria

The execution model is considered complete when:

- Sub-agents can retry predictably
- Escalations are deterministic
- Cleanup is safe and auditable
- Team mode and standalone mode share the same execution core
- All coordination rules are enforced in daemon RPC
- Future Web UI clients can implement orchestration purely through RPC calls without re-implementing logic
