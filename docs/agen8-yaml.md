# Project Desired State (`.agen8/agen8.yaml`)

This document explains the project-level desired-state manifest used by Agen8 to keep project teams running.

The file lives at:

```text
<project-root>/.agen8/agen8.yaml
```

`agen8 project init` creates this file automatically.

## What it controls

`.agen8/agen8.yaml` declares which teams a project should have running.

The daemon's project reconciler reads it and compares:

- desired teams from `.agen8/agen8.yaml`
- actual project teams from the runtime

When the daemon detects drift, it can:

- start a desired team that is missing
- recreate a desired team that has stale runtime state
- stop a reconciler-managed team that was removed from desired state or disabled

## Example

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: true
  - profile: market_researcher
    enabled: true
    heartbeat:
      overrideInterval: 30m
  - profile: reviewer_team
    enabled: false
```

## Schema

Top-level fields:

- `projectId`:
  - string
  - should match the project's initialized `projectId`
- `teams`:
  - list of desired project teams

Each `teams[]` item supports:

- `profile`:
  - string
  - required
  - the team profile to run
- `enabled`:
  - boolean
  - required
  - `true` means the team should be running
  - `false` means the team should not be running
- `heartbeat.overrideInterval`:
  - string
  - optional duration override such as `30m` or `1h`

All keys should use camelCase.

## Behavior

### `projectId`

If `projectId` is blank, Agen8 normalizes it to the project's default ID.

If `.agen8/agen8.yaml` contains a `projectId` that does not match the initialized project config, Agen8 reports an error when computing project drift or applying reconciliation.

### `teams`

Rules enforced today:

- each team entry must have a non-empty `profile`
- duplicate profiles are rejected case-insensitively
- blank `heartbeat.overrideInterval` values are ignored

If the file is missing, Agen8 currently treats desired state as:

```yaml
projectId: <project-id>
teams: []
```

In normal usage, `agen8 project init` creates the file so you should edit the generated manifest instead of relying on the implicit empty default.

## Common workflows

### Create the manifest

```sh
agen8 project init
```

That creates:

- `.agen8/config.toml`
- `.agen8/state.json`
- `.agen8/agen8.yaml`

### Check desired vs actual state

```sh
agen8 project status
```

`agen8 project status` shows:

- the manifest path
- whether the project is converged
- the current desired-state status
- any pending reconcile actions

Example fields:

```text
manifest=/path/to/project/.agen8/agen8.yaml
desired_converged=false
desired_status=drifting
action=spawn profile=dev_team team=- reason=desired team is missing
```

### Apply reconciliation immediately

```sh
agen8 project apply
```

`agen8 project apply` triggers an immediate reconcile pass instead of waiting for the daemon's periodic tick.

Example fields:

```text
applied=true
project_id=my-app
desired_converged=false
desired_status=reconciling
action=spawn profile=dev_team team=- reason=desired team is missing
```

### Enable a team

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: true
```

After the daemon reconcile tick, Agen8 should start the team automatically.

### Disable a team

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: false
```

If that team was created and managed by desired-state reconciliation, Agen8 should stop it on a later reconcile pass.

## Reconcile model

The daemon registers projects and periodically runs a reconcile pass.

At a high level it:

1. Reads `.agen8/agen8.yaml`
2. Loads project team records
3. Checks current runtime state
4. Computes drift
5. Starts, recreates, or stops teams as needed

Reconcile notifications are emitted with:

- `project.reconcile.started`
- `project.reconcile.drift`
- `project.reconcile.converged`
- `project.reconcile.failed`

These notifications drive the web convergence badges and activity feed updates.

## Notes and limitations

- `project.diff` and `project.apply` exist in the RPC layer.
- `agen8 project status` uses `project.diff`.
- `agen8 project apply` uses `project.apply`.
- Automatic stop behavior only applies to teams marked as reconciler-managed.
- `.agen8/config.toml` is still the runtime config file; `.agen8/agen8.yaml` is only for desired running state.

## Troubleshooting

If desired-state reconciliation is not behaving as expected, check:

1. the daemon is running:

   ```sh
   agen8 daemon status
   ```

2. the project is initialized:

   ```sh
   agen8 project status
   ```

3. the manifest path exists:

   ```sh
   ls .agen8/agen8.yaml
   ```

4. the manifest is valid YAML and uses camelCase keys
5. `projectId` matches the initialized project
6. the referenced `profile` names actually exist for the project/runtime

For raw runtime inspection, use:

```sh
agen8 logs --follow
```
