# Issue: Multi-team scheduling within a project

## Summary

A project can technically run multiple teams today, but the runtime has no concept of scheduling across them — there are no shared resource limits, no concurrency controls, and no way to prioritise one team's work over another's. This issue tracks adding a scheduler that manages multiple running teams as a single fleet.

## Problem

- Starting multiple teams is additive and uncontrolled. Each team consumes model calls and compute independently with no ceiling.
- There is no way to say "run at most 3 co-agents concurrently across all teams" or "dev_team gets priority over market_researcher."
- The operator has no single view of load across teams — only per-team visibility.

## Proposed approach

### 1. Project-level resource limits in the manifest

Extend `agen8.yaml` to express fleet-wide constraints:

```yaml
scheduler:
  max_concurrent_runs: 6      # across all teams in this project
  max_concurrent_per_team: 3  # per-team ceiling

teams:
  - profile: dev_team
    priority: high
  - profile: market_researcher
    priority: low
```

### 2. Scheduler component in the daemon

A scheduler sits between the reconciler and the runtime supervisor. When a team wants to start a new run (co-agent or sub-agent), it requests a slot from the scheduler rather than spawning directly.

- Slot allocation is based on current concurrency counts and declared priority.
- Low-priority teams are queued when the project ceiling is reached; high-priority teams preempt the queue.
- The scheduler exposes current slot state via RPC so the TUI and `project status` can surface it.

### 3. CLI surface

| Command | Behaviour |
|---|---|
| `agen8 project status` | Updated to show fleet-wide concurrency (slots used / total) |
| `agen8 logs` | New `scheduler.*` event types for slot grant, queue, and release |

## Acceptance criteria

- [ ] `max_concurrent_runs` is enforced across all teams in a project.
- [ ] `max_concurrent_per_team` is enforced per team independently.
- [ ] Runs that cannot start immediately are queued and start as slots free up.
- [ ] Priority ordering is respected when multiple queued runs compete for a slot.
- [ ] `agen8 project status` shows slot utilisation.
- [ ] Scheduler events appear in `agen8 logs`.

## Related

- `internal/app/team_daemon.go` — current team lifecycle management
- `docs/issues/desired-state-reconciliation.md` — reconciler that feeds work to the scheduler
- `pkg/services/session` — session start/stop the scheduler will gate
