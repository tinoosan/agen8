# Issue: Agent health probes and self-healing

## Summary

The runtime currently has no way to detect that an agent is stuck, looping, or silently failing. When an agent stops making progress, it either runs until a token budget is exhausted or sits indefinitely — requiring the operator to notice and intervene manually. This issue tracks adding liveness and progress probes that enable the daemon to detect and recover from unhealthy agents automatically.

## Problem

- An agent that is stuck in a reasoning loop consumes tokens indefinitely with no external signal.
- An agent waiting on a tool response that will never arrive (e.g. a hung subprocess) blocks its run slot with no timeout.
- The operator must watch `agen8 logs` or the TUI to notice a stalled agent. There is no automatic escalation.
- Heartbeats (scheduled wake-ups) exist per profile, but there is no equivalent probe for liveness of an *active* run.

## Proposed approach

### 1. Progress probe

Each active run has a progress deadline: if no tool call, event, or context update is emitted within a configurable window (`progress_timeout`), the daemon marks the run as `stalled`.

```yaml
# in profile or agen8.yaml
health:
  progress_timeout: 5m   # max silence before stall is declared
  max_consecutive_errors: 3  # LLM or tool errors before escalation
```

### 2. Stall recovery policy

When a run is marked `stalled`, the daemon applies a configurable recovery policy (in order):

1. **Warn** — emit a `run.stalled` event; notify operator via TUI and logs.
2. **Interrupt** — send a synthetic `task.interrupt` to the agent prompting it to summarise and yield.
3. **Restart** — terminate the run and start a fresh run with the same task (respects existing retry budget).
4. **Escalate** — if retries are exhausted, escalate to the parent or coordinator as a failed task.

### 3. Error threshold probe

Track consecutive LLM or tool errors per run. Exceeding `max_consecutive_errors` triggers the same recovery policy as a stall, starting from the interrupt step.

### 4. CLI surface

| Command | Behaviour |
|---|---|
| `agen8 logs` | New `run.stalled`, `run.interrupted`, `run.health.*` event types |
| `agen8 monitor` | Stalled runs surfaced visually (distinct from active/waiting) |
| `agen8 project status` | Health summary across all running teams |

## Acceptance criteria

- [ ] A run that emits no progress within `progress_timeout` is marked `stalled`.
- [ ] The daemon applies the configured recovery policy steps in order.
- [ ] An interrupted run receives a synthetic prompt to yield gracefully before being forcibly terminated.
- [ ] Restart respects the team's existing retry budget and escalation path.
- [ ] `run.stalled` and `run.health.*` events are emitted and visible in `agen8 logs`.
- [ ] The TUI monitor visually distinguishes stalled runs from healthy ones.
- [ ] All probe thresholds are configurable per-profile and can be overridden in `agen8.yaml`.

## Related

- `pkg/agent/session/session.go` — run lifecycle and heartbeat implementation (model for probe loop)
- `pkg/agent/loop.go` — agent loop where progress events originate
- `internal/app/team_daemon.go` — supervisor that would act on probe results
- `docs/execution-model.md` — escalation paths the recovery policy must follow
