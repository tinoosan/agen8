# Project Desired State (`.agen8/agen8.yaml`)

This document explains the project-level desired-state manifest used by Agen8 to keep project teams running.

`profile.yaml` defines what a team is. `.agen8/agen8.yaml` defines which teams should exist for a project.

## File location

The file lives at:

```text
<project-root>/.agen8/agen8.yaml
```

`agen8 project init` creates it automatically.

## Recommended project structure

Recommended layout:

```text
my-project/
├─ .agen8/
│  ├─ agen8.yaml
│  ├─ config.toml
│  ├─ state.json
│  ├─ README.md
│  └─ profiles/
│     └─ dev_team/
│        └─ profile.yaml
├─ src/
└─ README.md
```

Meaning of the files:

- `.agen8/agen8.yaml`
  - project desired running state
- `.agen8/config.toml`
  - project-local runtime defaults and overrides
- `.agen8/state.json`
  - active session/team/run pointers
- `.agen8/profiles/`
  - optional project-owned profile source files

Important current behavior:

- desired-state `profile` refs should normally be canonical profile IDs such as `dev_team`
- bare profile IDs resolve from `${AGEN8_DATA_DIR}/profiles/<profile-id>/profile.yaml`
- project-local `.agen8/profiles/` are useful project source/config, but they are not yet the primary bare-name resolution source for desired-state reconciliation

## What it controls

`.agen8/agen8.yaml` declares which teams the project should have.

The daemon reconciler compares:

- desired teams from `.agen8/agen8.yaml`
- actual project teams from the runtime and project team registry

When it detects drift, it can:

- start a missing desired team
- recreate a managed desired team when runtime state is stale
- recreate a managed desired team when its `profile.yaml` changed
- stop a managed desired team when it is still declared but `enabled: false`
- delete a managed desired team when it is removed from the manifest

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

- `projectId`
  - string
  - should match the initialized project `projectId`
- `teams`
  - list of desired teams

Each `teams[]` item supports:

- `profile`
  - string
  - required
  - recommended: canonical profile ID such as `dev_team`
- `enabled`
  - boolean
  - required
- `heartbeat.overrideInterval`
  - string
  - optional duration override such as `30m` or `1h`

All keys should use camelCase.

## How `profile` is interpreted

Recommended:

```yaml
teams:
  - profile: dev_team
    enabled: true
```

That tells Agen8 to resolve:

```text
${AGEN8_DATA_DIR}/profiles/dev_team/profile.yaml
```

Why this matters:

- the reconciler stores and compares teams by resolved profile ID
- rollout detection fingerprints the resolved `profile.yaml`
- canonical profile IDs give stable matching between desired state and actual runtime state

Not recommended in `.agen8/agen8.yaml`:

- raw ad hoc file paths
- refs that do not match the profile's own `id`

## Behavior

### `projectId`

If `projectId` is blank, Agen8 normalizes it to the project default ID.

If `.agen8/agen8.yaml` contains a `projectId` that does not match the initialized project config, Agen8 reports an error during diff/apply/reconcile.

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

In normal usage you should edit the generated file, not rely on the implicit empty default.

## Relationship to `profile.yaml`

`profile.yaml` defines:

- roles
- coordinator selection
- reviewer
- team model and per-role models
- prompts
- heartbeat jobs
- replica hints

`.agen8/agen8.yaml` defines:

- which profile-backed teams should exist for this project
- whether each should be enabled
- optional desired-state heartbeat override interval

Short version:

- `profile.yaml` = template/spec for one team type
- `.agen8/agen8.yaml` = desired inventory of teams for the current project

## Profile change rollouts

Managed desired-state teams now track a fingerprint of the resolved `profile.yaml`.

That means:

- if a managed team's `profile.yaml` changes
- and that team is still desired in `.agen8/agen8.yaml`
- the reconciler marks it for `recreate`

Current rollout detection scope:

- tracked: resolved `profile.yaml` content
- not yet tracked: external prompt fragment files, `prompt.md`, or other referenced files

## Reconcile semantics

Current behavior:

- desired + missing => `spawn`
- desired + stale/not running => `recreate`
- desired + `profile.yaml` changed => `recreate`
- desired + `enabled: false` => `stop`
- removed from manifest => `delete`

The last case is intentionally Kubernetes-like for managed teams:

- removing a team from `.agen8/agen8.yaml` deletes it
- setting `enabled: false` keeps the team definition but stops it

`recreate` is not the same as `delete`:

- `recreate` preserves old session/run history and replaces the active team session
- `delete` removes the managed team and its team-scoped runtime data

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

Example output fields:

```text
manifest=/path/to/project/.agen8/agen8.yaml
project_id=my-app
desired_converged=false
desired_status=drifting
action=spawn profile=dev_team team=- reason=desired team is missing
```

### Apply reconciliation immediately

```sh
agen8 project apply
```

This triggers an immediate reconcile pass instead of waiting for the periodic daemon tick.

Example output:

```text
applied=true
project_id=my-app
desired_converged=false
desired_status=reconciling
action=recreate profile=dev_team team=team-123 reason=desired profile.yaml changed
```

### Add a desired team

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: true
```

Expected result:

- daemon starts the team if missing
- project status eventually converges

### Temporarily disable a team

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: false
```

Expected result:

- managed team is stopped
- team is not deleted

### Remove a team entirely

```yaml
projectId: my-app

teams: []
```

Expected result:

- managed team is deleted
- project team record, sessions, and team directory are cleaned up

## Related configuration

Files that matter for project/team configuration:

- `.agen8/agen8.yaml`
  - desired running state for the project
- `.agen8/config.toml`
  - project-local runtime defaults like RPC endpoint or data-dir override
- `${AGEN8_DATA_DIR}/config.toml`
  - global runtime defaults
- `${AGEN8_DATA_DIR}/profiles/<id>/profile.yaml`
  - shared profile definitions
- `.agen8/profiles/...`
  - optional project-owned profile source files

See also:

- [docs/profile-yaml.md](profile-yaml.md)
- [docs/config-toml.md](config-toml.md)

## Troubleshooting

If desired-state reconciliation is not behaving as expected:

1. confirm the daemon is running:

   ```sh
   agen8 daemon status
   ```

2. confirm the project is initialized:

   ```sh
   agen8 project status
   ```

3. confirm the manifest exists:

   ```sh
   ls .agen8/agen8.yaml
   ```

4. confirm `projectId` matches the initialized project
5. confirm all `profile` values refer to real canonical profiles
6. after editing a profile, run:

   ```sh
   agen8 project apply
   ```

7. inspect logs:

   ```sh
   agen8 logs --follow
   ```
