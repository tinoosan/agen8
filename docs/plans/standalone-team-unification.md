# Plan: Standalone = Team with One Agent + Configurable Sub-agents

## 1. Other Boundaries (Current State)

### Boundaries already addressed
- **Webhook** → `internal/webhook` (HTTP, JSON, task ingester)
- **RPC storage** → `internal/storage` (PlanReader, FileReader, WorkspacePreparer)
- **Session** → `pkg/services/session` (single entry point for session/run data)
- **TUI/CLI** → use session service, not `internal/store`

### Remaining boundary considerations

| Area | Current | Recommendation |
|------|---------|-----------------|
| **Daemon vs Team daemon** | Two entry points: `RunDaemon` (standalone) vs `runAsTeam` (team). Different bootstrap, different runtime supervisors. | Unification (below) collapses this. |
| **Profile resolution** | `resolveProfileRef` in app; profile loading scattered. | Consider `pkg/profile` as single loader; app only resolves by ref. |
| **Task store** | `pkg/agent/state` (TaskStore) used directly by daemon, RPC, session. | Task service exists (`pkg/services/task`); ensure all task access goes through it. |
| **Event emission** | Events emitted from many layers (daemon, agent, RPC). | Events service centralizes; verify no direct event bus usage outside it. |
| **Config / runtime config** | `config.Config`, runtime config files, env. | Already centralized; ensure no ad-hoc config parsing. |

---

## 2. Standalone = Team with One Agent

### Current model
- **Standalone**: `sess.TeamID == ""`, `sess.Mode == "standalone"`. Single run. Profile has no `team` block. `spawn_worker` enabled.
- **Team**: `sess.TeamID != ""`, `sess.Mode == "team"`. Multiple roles, team manifest, coordinator. `spawn_worker` disabled. Subagents tab hidden in TUI.

### Target model
- **All sessions are teams.** Standalone = team with exactly one role.
- Single code path: `sessionStart` always creates a team (1+ roles).
- Profile structure: team profiles with `roles`; "standalone" profiles become team profiles with one role.

### Migration approach

#### Phase A: Profile representation
1. **Standalone profiles** → treat as team with one role:
   - `general` profile: add `team: { roles: [{ name: "agent", coordinator: true, ... }] }` derived from top-level fields, or
   - Keep backward compat: profile without `team` is normalized to `team.roles: [single role from top-level]` at load time.
2. **Validation**: Require exactly one coordinator. Single-role team = valid.

#### Phase B: Session start
1. **Unify `sessionStart` and `sessionStartTeam`**:
   - One `sessionStart` that always creates a team (teamID, manifest, workspace).
   - For single-role profile: create team with one role, one run. Same manifest/workspace structure.
2. **Session fields**: `sess.TeamID` always set (e.g. `team-<uuid>`). `sess.Mode` can stay for display ("standalone" vs "team") or become derived: `mode = len(roles) == 1 ? "standalone" : "team"`.

#### Phase C: Daemon / runtime
1. **Single daemon path**: `RunDaemon` always uses team flow. `runAsTeam` becomes the only path; standalone is `runAsTeam` with profile that has one role.
2. **Runtime supervisor**: One supervisor type. Team supervisor already handles N roles; N=1 is a special case.
3. **Legacy cleanup**: Remove "team mode does not support child runs" special case once subagents are configurable (Phase D).

#### Phase D: TUI
1. **Monitor**: Always team-based. `teamID` always set. Single-role team: hide multi-role UI (role picker, etc.), show Subagents tab if `allow_subagents`.
2. **Wizard**: "New Standalone" = pick single-role profile. "New Team" = pick multi-role profile. Or: one "New Session" that picks profile; mode is inferred.

---

## 3. Configurable Sub-agents in profile.yaml

### Requirement
- Control whether agents in a team can spawn sub-agents.
- Apply to coordinator and/or workers.
- Define in `profile.yaml`.

### Chosen schema: Per-role, default false

```yaml
team:
  roles:
    - name: ceo
      coordinator: true
      allow_subagents: true   # coordinator can spawn (explicit opt-in)
      ...
    - name: cto
      allow_subagents: true   # worker can spawn
      ...
    - name: designer
      # omit or false: cannot spawn (default)
      ...
```

- **Per-role only**: Each role has its own `allow_subagents` setting.
- **Default: false** when omitted. Coordinator and workers must explicitly opt in.
- No team-level default; each role is independent.
- **Standalone profile normalization**: When converting a non-team profile to a single-role team, the synthetic role may default `allow_subagents: true` to preserve current standalone spawn behavior (or false if we accept that as a breaking change).

### Implementation points

1. **Profile struct** (`pkg/profile/profile.go`):
   - `RoleConfig.AllowSubagents bool` (default false when omitted)

2. **Runtime supervisor** (`daemon_runtime_supervisor.go`):
   - Replace `teamID != ""` → no spawn with: check `role.AllowSubagents` for the current run's role.
   - Standalone (single-role): resolve from that role's `AllowSubagents`.

3. **TaskCreateTool** / `makeSpawnWorkerFunc`:
   - Pass `allowSubagents bool` instead of inferring from `teamID == ""`.
   - When false, `SpawnWorker` is nil.

4. **TUI Subagents tab**:
   - Show when `allow_subagents` is true for the current run’s role (or team).
   - Remove "hide in team mode" rule; use profile config.

5. **Legacy team child runs**:
   - Remove "team mode does not support child runs" cleanup once config-driven.
   - Existing team sessions: roles without `allow_subagents: true` keep current behavior (no spawn).

---

## 4. Pre-implementation: Preparation

Before changing code, ensure the following are in place so we can execute safely and roll back if needed.

### 4.1 Impact surface (inventory)

**Search targets before starting**: `teamID == ""`, `teamID != ""`, `strings.TrimSpace(m.teamID)`, `sess.TeamID`, `Mode == "standalone"`, `Mode == "team"`, `prof.Team == nil`, `spawn_worker`, `makeSpawnWorkerFunc`, `ParentRunID == ""`.

| Layer | Files / areas | What changes |
|-------|---------------|--------------|
| **Profile** | `pkg/profile/profile.go` | Add `RoleConfig.AllowSubagents`; optional: normalize standalone → single-role at load |
| **Daemon** | `internal/app/daemon.go`, `daemon_builder.go`, `team_daemon.go` | Collapse to single path; `RunDaemon` → `runAsTeam` with profile |
| **Runtime supervisor** | `internal/app/daemon_runtime_supervisor.go` | Gate spawn by `role.AllowSubagents`; remove `teamID != ""` spawn block |
| **Team runtime** | `internal/app/team_daemon.go`, `team_state.go` | May absorb standalone; or standalone uses team supervisor with N=1 |
| **RPC session** | `internal/app/rpc_session.go` | Merge `sessionStart` + `sessionStartTeam`; always create team |
| **RPC team** | `internal/app/rpc_team.go` | May need to handle single-role teams (manifest, workspace) |
| **Storage** | `internal/storage/`, `internal/store/` | Workspace layout for single-role? Session/run schema unchanged |
| **TUI monitor** | `internal/tui/monitor*.go` | Subagents tab from profile; remove `teamID == ""` branches; wizard flow |
| **CLI** | `cmd/agen8/cmd/*.go` | Mode flags, dashboard, workflow init – may need updates |
| **Protocol** | `pkg/protocol/` | Session start params; mode semantics |
| **Agent session** | `pkg/agent/session/` | Callback handling, task sources – likely minimal if team structure is same |

### 4.2 Pre-requisites checklist

- [ ] **Test baseline**: Run full test suite, record pass/fail. Fix or document known flaky tests (e.g. `TestLoadPlanFilesCmd_TeamFocusedLoadsSingleRunPlan`).
- [ ] **Integration test**: Add or document a test that covers: standalone session start → run → spawn_worker → callback. Use as regression guard.
- [ ] **Profile test**: Add test for `allow_subagents` parsing (true, false, omitted → false).
- [ ] **Branch strategy**: Work on a feature branch; keep main deployable. Consider smaller PRs per phase.
- [ ] **Rollback plan**: Each phase should be independently revertible. Avoid mixing `allow_subagents` wiring with unification in one commit.
- [ ] **Documentation**: Update `docs/architecture/` and `docs/webhooks.md` (if affected). Document new profile schema in `docs/config-toml.md` or profile docs.

### 4.3 Incremental rollout

1. **Phase 0 (prep)**: Add `allow_subagents` to profile struct only. No behavior change. Deploy, verify.
2. **Phase 1a**: Wire `allow_subagents` into runtime supervisor; gate spawn by it. Keep `teamID == ""` as secondary condition (both must allow). Deploy, verify team still works, standalone still works.
3. **Phase 1b**: Remove `teamID == ""` spawn logic; rely only on `allow_subagents`. Update TUI Subagents tab. Deploy, verify.
4. **Phase 2**: Unify session start. Standalone profile → single-role team at load. Merge RPC handlers. Deploy, verify.
5. **Phase 3**: Unify daemon. Single `runAsTeam` path. Deploy, verify.
6. **Phase 4**: TUI cleanup. Remove legacy `teamID == ""` branches. Wizard simplification.

### 4.4 Risk controls

- **Feature flag** (optional): `AGEN8_UNIFIED_TEAM=1` to opt into new session start path. Default off until validated.
- **Data migration**: No DB schema change. Old sessions with `TeamID == ""` continue to work; runtime treats them as single-role when needed.
- **Profile compatibility**: Standalone profiles (no `team` block) normalized at load time only; no file edits required for existing profiles.

---

## 5. Implementation Order (detailed)

1. **Add `allow_subagents` to profile** (per-role, default false)
   - Extend `RoleConfig` with `AllowSubagents bool`
   - Wire into runtime supervisor; gate `spawn_worker` by role config instead of `teamID == ""`
   - Update TUI: Subagents tab visibility from profile, not `teamID`

2. **Unify session start** ✅ (Phase 2 done)
   - Profile.RolesForSession() normalizes standalone → single role "agent"
   - Profile.TeamModelForSession() for model resolution
   - Single sessionStart flow; always create teamID, manifest, workspace
   - Mode = "standalone" when 1 role, "team" when 2+

3. **Unify daemon**
   - Single `runAsTeam` path; standalone = team with one role
   - Remove `RunDaemon` vs `runAsTeam` branching at top level

4. **TUI simplification**
   - Remove `teamID == ""` branches where possible
   - Mode = derived from role count
   - Wizard: one flow, mode from profile

---

## 6. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing standalone sessions | Migration: sessions with `TeamID == ""` treated as single-role team at runtime; no schema change for old data. |
| Breaking existing team sessions | No change to team session structure; only add `allow_subagents` (default false). |
| Profile migration | Standalone profiles: add synthetic `team` block at load time; no file changes required. |
| TUI regression | Feature-flag or gradual rollout; keep both code paths until validated. |

---

## 7. Open Questions

1. **Naming**: Keep "standalone" and "team" as display concepts, or rename to "single-agent" / "multi-agent"?
2. **Single-role team workspace**: Same layout as current standalone (no `teams/<id>/`?) or always use team workspace layout for consistency?
