---
name: agen8-graph
description: Use to read the Agen8 work graph with graph_query — see how a mission, its key results, tasks, and decisions connect, read context after compaction or before deciding, and (the part agents skip) actively link related work so the richer relationships exist to find later.
---

# Agen8 Graph

The Agen8 graph is the project's work memory. Nodes are work entities — `mission`, `key_result`, `task`, `decision`. `graph_query` reads and writes the edges between them. It's a context graph of *work*, not a log of tool calls or spend.

There are **two kinds of edge**, and the difference is the whole point of this skill:

1. **Structural edges — derived from refs at read time, for free.** When a task carries a `key_result_ref`/`mission_ref`, or a decision carries a `task_ref`/`key_result_ref`/`mission_ref`, the graph computes the edge when you read the node. You never create these; they appear automatically. Only two types are derived:
   - `serves` — task → KR (or → mission), decision → KR/mission, KR → mission.
   - `made_during` — decision → the task it was logged against.
   This is a **tree**: mission → key results → tasks → decisions. It is most of what you will see, and it exists only because refs were set correctly.

2. **Manual links — stored only if an agent creates them with `action=link`.** Every other edge type (`blocked_by`, `informed_by`, `supersedes`, `relates_to`, `resolved_by`, `completed_by`, `produced`, `spawned`, `child_of`) is valid but **nothing in the system emits it**. If you want the graph to show that one decision superseded another, or that a task is blocked by another, you have to link it. Otherwise that relationship does not exist for the next agent to find.

The common failure is treating the graph as if it's richly typed when, by default, it's a `serves`/`made_during` skeleton. Reading it is half the job; **keeping it connected is the other half.**

## Reach For The Graph When

- **Before choosing or starting work** — expand the mission or KR to see its tree: which tasks and decisions hang off it, and where the open work is.
- **After compaction or when joining ongoing work** — the graph is the durable structure; rebuild your picture from it, not from compressed chat.
- **During review** — see what a node `serves` and what it was `made_during`, so you judge the work in its place, not in isolation.
- **Before re-deciding** — search past decisions by keyword. If you find one this replaces, link `supersedes` so it doesn't keep reading as current.

## Reading

`graph_query action=node` is the workhorse. Give it a `node_id` (and `node_type` if the id isn't prefixed) plus `depth` (1–3). It returns the node, its neighbours, and the edges — both the derived tree and any manual links. Depth 1 is direct neighbours; raise it to walk the tree further. Use it to answer "what does this connect to?"

`graph_query action=search` finds nodes by `node_type` + `query` (keyword over titles). Use it to locate a decision/task/mission before expanding it with `node`.

**Edge filters on search match stored manual links only — not the derived tree.** `has_edge`, `missing_edge`, `outgoing_edge`, `incoming_edge` look at links an agent created, so they're useful for finding manual relationships (e.g. `has_edge=supersedes` to find decisions someone marked as superseding another). Do **not** use `missing_edge=serves` to find "unanchored" tasks — `serves` is derived, not stored, so that filter won't mean what it looks like. To check anchoring, expand the node and look for the `serves` edge, or read the task's `keyResultRef`/`missionRef` directly.

## Keep The Graph Connected

The structural tree is automatic, but the relationships that make the graph worth traversing are not. When a real relationship exists and isn't a parent/child `serves`, record it with `action=link` (`source_id`/`source_type`, `target_id`/`target_type`, `edge_type`, and a real `rationale`):

- A decision that **replaces** an earlier one → `supersedes`. Do this whenever you re-decide; a stale decision left unlinked reads as still-current.
- A decision that **built on** another → `informed_by`.
- Work **blocked by** other work → `blocked_by`.
- A genuine relationship with no stronger fit → `relates_to`.

Write a `rationale` that tells a later reader why; "decision serves KR" tells them nothing. The `serves`/`made_during` edges are created for you from refs, so the manual ones you add are exactly the cross-links the tree can't capture.

## Keep It Honest

- Don't invent nodes, edges, or relationships. If a query returns nothing, that is the answer — say so, rather than narrating a graph that isn't there.
- Recalled graph state reflects when it was read; verify a node still exists before acting on an old reference.
- Prefer member display names and clear titles over raw ids in anything a human will read.
