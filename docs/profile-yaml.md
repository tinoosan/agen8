# Team Profiles (`profile.yaml`)

This document explains how Agen8 team profiles work, where they live, what a `profile.yaml` can contain, and how profiles interact with `.agen8/agen8.yaml`.

## What a profile does

A `profile.yaml` defines the shape of a team or standalone agent:

- identity: `id`, `name`, `description`
- model defaults
- prompts and prompt fragments
- skills and tool restrictions
- heartbeat jobs
- team roles, coordinator selection, reviewer, and replica hints

In practice:

- `profile.yaml` defines what a team is
- `.agen8/agen8.yaml` defines which teams should be running for a project

## Recommended directory structure

Recommended project layout:

```text
my-project/
├─ .agen8/
│  ├─ config.toml
│  ├─ state.json
│  ├─ agen8.yaml
│  └─ profiles/
│     └─ dev_team/
│        ├─ profile.yaml
│        ├─ prompt.md
│        └─ roles/
│           └─ reviewer.yaml
├─ app/
└─ README.md
```

Recommended shared runtime profile layout:

```text
${AGEN8_DATA_DIR}/profiles/
├─ general/
│  └─ profile.yaml
├─ dev_team/
│  ├─ profile.yaml
│  └─ prompt.md
└─ market_researcher/
   └─ profile.yaml
```

## Where profiles are resolved from

For bare profile refs such as `dev_team`, Agen8 resolves profiles from:

```text
${AGEN8_DATA_DIR}/profiles/<profile-ref>/profile.yaml
```

The runtime also accepts an explicit filesystem path to a profile directory or `profile.yaml` file in some entrypoints.

Recommended usage:

- use canonical profile IDs like `dev_team`
- store shared reusable profiles under `${AGEN8_DATA_DIR}/profiles/`
- treat project-local `.agen8/profiles/` as project-owned source/config content, not the primary desired-state reference target unless you are intentionally using explicit paths
- if you want guaranteed bare-name resolution today, put the active profile under `${AGEN8_DATA_DIR}/profiles/<id>/profile.yaml`

Important current behavior:

- desired-state reconciliation matches teams by resolved profile ID
- because of that, `.agen8/agen8.yaml` should use canonical profile IDs, not ad hoc path refs
- if you use a path ref in `.agen8/agen8.yaml`, the current reconciler may not match it stably against the stored team record

## Minimal example

```yaml
id: dev_team
description: Product engineering team

team:
  model: openai/gpt-5
  roles:
    - name: coordinator
      coordinator: true
      description: Coordinates the team and talks to the user
      prompts:
        systemPrompt: Coordinate work across the team.

    - name: engineer
      description: Implements code changes
      prompts:
        systemPrompt: Implement and validate code changes.
```

## Standalone vs team profiles

### Standalone profile

If `team:` is omitted, Agen8 treats the profile as a standalone/single-role profile.

Example:

```yaml
id: general
description: General-purpose agent
model: openai/gpt-5-mini
prompts:
  systemPrompt: Be a practical general-purpose agent.
skills:
  - coding
```

This is normalized into a single coordinator-style role internally.

### Team profile

If `team:` is present, Agen8 expects explicit team role definitions.

Example:

```yaml
id: dev_team
description: Software development team

team:
  model: openai/gpt-5
  reviewer:
    enabled: true
    description: Reviews work before completion
    prompts:
      systemPrompt: Review changes for bugs and regressions.
  roles:
    - name: coordinator
      coordinator: true
      description: Coordinates work
      prompts:
        systemPrompt: Coordinate the team.

    - name: engineer
      description: Writes code
      prompts:
        systemPrompt: Write and validate code.
      replicas: 2
```

## Schema overview

Common top-level fields:

- `id`: required unique profile ID
- `name`: optional display name
- `description`: required summary
- `model`: optional default model
- `subagentModel`: optional default sub-agent model
- `codeExecOnly`: optional boolean
- `codeExecRequiredImports`: optional list
- `allowedTools`: optional allowlist
- `skills`: optional list of skill names/refs
- `prompts`: prompt configuration
- `heartbeat`: optional heartbeat configuration
- `team`: optional team configuration

### `prompts`

Supported forms:

- `prompts.systemPrompt`
- `prompts.systemPromptPath`
- `prompts.systemFragments`

If no prompt is configured and `prompt.md` exists in the same profile directory, Agen8 uses `prompt.md` as the default system prompt source.

Rules:

- `systemPrompt/systemPromptPath` cannot be mixed with `systemFragments`
- `systemPromptPath` must stay relative to the profile directory
- fragment paths must be relative

### `heartbeat`

Supported forms:

```yaml
heartbeat:
  enabled: true
  jobs:
    - name: daily-digest
      interval: 24h
      goal: Summarize important changes.
```

Or legacy list form:

```yaml
heartbeat:
  - name: daily-digest
    interval: 24h
    goal: Summarize important changes.
```

### `team.roles`

Rules enforced by the loader:

- `team.roles` must contain at least one role
- exactly one role must set `coordinator: true`
- role names must be unique case-insensitively
- each role requires `name`, `description`, and prompt configuration
- `replicas` must be `>= 1`
- coordinator roles cannot set `replicas`

### `team.reviewer`

Optional reviewer configuration:

- `enabled: true` turns it on
- reviewer name defaults to `reviewer`
- description is required when reviewer is enabled

## Relationship to `.agen8/agen8.yaml`

Example desired-state manifest:

```yaml
projectId: my-app

teams:
  - profile: dev_team
    enabled: true
```

How it works:

1. `.agen8/agen8.yaml` names a desired profile ref such as `dev_team`
2. the runtime resolves that to a `profile.yaml`
3. the profile defines the actual team model, roles, reviewer, prompts, heartbeat jobs, and replica hints
4. the reconciler ensures that project teams match the desired set

## Profile updates and rollouts

Desired-state reconciliation now fingerprints the resolved `profile.yaml` file for managed teams.

That means:

- if `profile.yaml` changes
- and the team is managed through `.agen8/agen8.yaml`
- the reconciler marks that team for `recreate`

Current scope of rollout detection:

- tracked: the resolved `profile.yaml` file contents
- not yet tracked: referenced prompt fragment files, `prompt.md`, or other indirect files

Recommended operator flow:

1. edit `profile.yaml`
2. run:

   ```sh
   agen8 project apply
   ```

3. verify drift/convergence:

   ```sh
   agen8 project status
   ```

## Recommended conventions

- Keep `id` stable. Treat it like a deployment name.
- Use canonical IDs such as `dev_team`, `reviewer_team`, `market_researcher`.
- Prefer bare profile IDs in `.agen8/agen8.yaml`.
- Keep prompt files next to the profile that uses them.
- Use `prompt.md` only when you want the default prompt behavior.
- Put shared reusable profiles in `${AGEN8_DATA_DIR}/profiles`.
- Put project-only profile source files under `.agen8/profiles` if you want them versioned with the project, but prefer promoting actively used profiles into the shared profile directory until project-local profile ref resolution is unified.

## Troubleshooting

If a profile is not behaving as expected:

1. confirm the profile exists under `${AGEN8_DATA_DIR}/profiles/<id>/profile.yaml`
2. confirm `id:` matches the intended canonical profile ID
3. confirm required prompts exist
4. confirm exactly one team role is marked `coordinator: true`
5. after changing `profile.yaml`, run:

   ```sh
   agen8 project apply
   ```

6. inspect live state:

   ```sh
   agen8 project status
   agen8 team list
   agen8 logs --follow
   ```
