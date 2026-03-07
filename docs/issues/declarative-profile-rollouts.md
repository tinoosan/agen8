# Issue: Declarative profile rollouts

## Summary

Updating a running team today requires a full stop and restart — there is no way to roll out a profile change (new model, updated system prompt, added skill) while the team continues processing work. This issue tracks a rollout mechanism that applies profile updates gracefully, draining in-flight work before swapping the configuration.

## Problem

- `team start <profile>` is all-or-nothing. A profile change means stopping the team, losing in-flight context, and restarting cold.
- Operators cannot test a profile change on one co-agent while the rest of the team runs the previous version.
- There is no audit trail of which profile version was running when a specific run was executed.

## Proposed approach

### 1. Profile versioning

Profiles gain a `version` field (or derived from a content hash if not set). The daemon records the profile version alongside each run in the store, giving full auditability of what configuration produced what output.

### 2. Rollout strategies in the manifest

`agen8.yaml` supports a `rollout` stanza per team:

```yaml
teams:
  - profile: dev_team
    rollout:
      strategy: drain        # wait for in-flight runs to complete, then swap
      # strategy: immediate  # terminate runs and restart immediately
      # strategy: canary     # apply to one replica first, then roll out
```

### 3. Rollout flow (`drain` strategy)

1. Operator updates `profile.yaml` or bumps a version reference in `agen8.yaml`.
2. `agen8 project apply` (or the reconciler on the next tick) detects a version mismatch.
3. The daemon marks affected agent instances as `draining` — they complete their current task but accept no new ones.
4. As each instance drains, it is restarted with the updated profile.
5. Once all instances are on the new version, the rollout is marked complete.

### 4. Canary strategy

Apply the new profile to one replica (for multi-replica roles). Monitor its runs for a configurable window. If no escalations or stalls occur, roll out to remaining replicas. If the canary fails, the rollout is aborted and the instance is reverted.

### 5. CLI surface

| Command | Behaviour |
|---|---|
| `agen8 project apply` | Triggers rollout if profile versions differ from running state |
| `agen8 project status` | Shows rollout progress (e.g. `2/3 replicas on v2`) |
| `agen8 logs` | New `rollout.*` event types for drain, swap, canary result |

## Acceptance criteria

- [ ] Profile version is recorded per run in the store.
- [ ] `drain` strategy waits for in-flight runs to complete before restarting instances with the new profile.
- [ ] `immediate` strategy terminates and restarts without waiting.
- [ ] `canary` strategy applies the update to one replica first and gates the rest on a health window.
- [ ] A failed canary aborts the rollout and leaves remaining instances on the previous profile version.
- [ ] `agen8 project status` reports rollout progress.
- [ ] Rollout events are emitted and visible in `agen8 logs`.

## Related

- `internal/app/team_daemon.go` — team and agent instance lifecycle
- `defaults/profiles/` — profile definitions that will gain versioning
- `docs/issues/desired-state-reconciliation.md` — reconciler that detects version drift and triggers rollouts
- `docs/issues/agent-health-probes.md` — health signal used by canary gating
