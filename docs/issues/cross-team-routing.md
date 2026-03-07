# Issue: Cross-team task routing

## Summary

Tasks today are scoped to a single team. There is no mechanism for one team to send work to another team within the same project — if `dev_team` needs market research, a human must relay that manually. This issue tracks a cross-team routing primitive that lets teams delegate work to each other.

## Problem

- The execution model explicitly forbids lateral communication between agents within a team (no sibling-to-sibling messaging). This is correct for intra-team hygiene.
- But there is no sanctioned path for *inter-team* work handoffs at the project level either.
- Teams that naturally produce outputs another team consumes (e.g. market research feeding product decisions) require manual operator intervention to connect.

## Proposed approach

### 1. Team-addressed task routing

Extend the `task_create` host tool to accept a `team` target in addition to `spawn_worker`:

```
task_create(
  goal: "Research competitor pricing for Q2",
  route_to_team: "market_researcher",
  callback: true
)
```

The runtime resolves `route_to_team` to the named team's coordinator inbox and delivers the task there. The originating team's coordinator receives a callback when the work completes, exactly as sub-agent callbacks work today.

### 2. Routing table in the daemon

The daemon maintains a project-level routing table: team name → active session ID. The reconciler (see `desired-state-reconciliation.md`) keeps this table current as teams start and stop.

Cross-team tasks are held in a `pending_route` state if the target team is not currently running, and delivered when it starts.

### 3. Authority and trust model

Cross-team routing is peer-to-peer at the coordinator level — a coordinator can request work from another team's coordinator, but cannot issue commands into that team's internal hierarchy. The receiving coordinator decides how to handle the incoming task and may decline or defer it.

This preserves the tree-based authority model within each team while allowing project-level coordination between them.

### 4. CLI surface

| Command | Behaviour |
|---|---|
| `agen8 task list --project` | Show tasks across all teams, including cross-team ones |
| `agen8 logs` | New `task.route.*` event types for cross-team delivery |

## Acceptance criteria

- [ ] A coordinator can issue `task_create` with `route_to_team` targeting another team in the same project.
- [ ] The routed task appears in the target team's coordinator inbox.
- [ ] A callback is delivered to the originating coordinator when the target team completes the task.
- [ ] Tasks routed to a team that is not running are queued and delivered on start.
- [ ] The receiving coordinator retains full authority over how to handle the incoming task.
- [ ] `agen8 task list --project` surfaces cross-team tasks with their routing state.

## Related

- `pkg/agent/hosttools/task_create.go` — host tool to extend
- `docs/execution-model.md` — authority model this must not violate
- `docs/issues/desired-state-reconciliation.md` — routing table depends on reconciler knowing what teams are running
- `pkg/protocol/protocol_test.go` — existing inter-team protocol tests
