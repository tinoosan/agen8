# Issue: Desired state reconciliation for projects

## Summary

Today a project is activated imperatively — `agen8 team start <profile>` boots a team and it runs until stopped. There is no persistent record of what *should* be running, so restarts are manual and drift between intended and actual state is invisible.

This issue tracks adding a project-level desired state manifest and a reconciler that continuously enforces it.

## Problem

- `team start` is a one-shot imperative command. If the daemon restarts or a team exits unexpectedly, nothing brings it back.
- There is no way to declare "this project should always have a `dev_team` running" and have the runtime uphold that contract.
- Operators must manually detect and recover from team failures.

## Proposed approach

### 1. Project manifest (`agen8.yaml`)

Add an `agen8.yaml` file at the project root (or inside `.agen8/`) that declares the desired running state:

```yaml
project: my-app

teams:
  - profile: dev_team
    enabled: true
  - profile: market_researcher
    enabled: true
    heartbeat:
      override_interval: 30m
```

`project init` writes an initial manifest. `project status` diffs desired vs. actual.

### 2. Reconciler loop

A reconciler runs inside the daemon on a short poll interval (e.g. 15s). On each tick it:

1. Reads the project manifest.
2. Queries the runtime supervisor for currently running sessions.
3. For each desired team that is not running → starts it.
4. For each running team not in the manifest (or with `enabled: false`) → stops it gracefully.

The reconciler emits structured events (`project.reconcile.start`, `project.reconcile.drift`, `project.reconcile.converged`) visible in `agen8 logs`.

### 3. CLI surface

| Command | Behaviour |
|---|---|
| `agen8 project apply` | Force an immediate reconciliation pass |
| `agen8 project diff` | Show desired vs. actual without acting |
| `agen8 project status` | Existing command, updated to show convergence state |

## Acceptance criteria

- [ ] `agen8.yaml` is read by the daemon on startup and on each reconciler tick.
- [ ] A team listed in the manifest as `enabled: true` is automatically (re)started if not running.
- [ ] A team removed from the manifest (or set `enabled: false`) is stopped gracefully.
- [ ] `agen8 project diff` outputs a human-readable drift report.
- [ ] Reconciler events appear in `agen8 logs`.
- [ ] Daemon restart with an existing manifest converges to desired state without manual intervention.

## Related

- `internal/app/team_daemon.go` — team lifecycle management
- `pkg/services/session` — session start/stop
- `docs/execution-model.md` — authority model the reconciler must respect
