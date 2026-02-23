# Plan: Monitor – Everything Is a Team (Redesign)

## Principle

**Everything is a team.** No backward compatibility for "standalone mode". No special cases. This is a redesign to simplify the model and prevent regressions.

- Every session has a team (1 or N agents).
- The monitor always operates in team context.
- Remove all code that branches on "standalone" or treats `teamID == ""` as a valid state for active sessions.

## What to Remove

| Pattern | Action |
|---------|--------|
| `teamID == ""` branches | Remove. Team is always present for active sessions. |
| `Mode == "standalone"` / `Mode == "single-agent"` special handling | Remove. Mode is display-only (1 vs N agents). |
| "only available in multi-agent monitor" | Remove. `/team` works for all sessions. |
| Fallbacks for "legacy" or "no team" | Remove. No backward compat. |
| `RunMonitor(runID)` without loading team | Change: always load team from `run.Runtime.TeamID`. |
| Dual paths: run-focused vs team-focused | Unify: one model, team context always loaded; runID = initial focus. |

## What to Keep / Use

- Team manifest, `loadTeamManifest`, team workspace layout
- `monitorSwitchTeamMsg` / `monitorSwitchRunMsg` for in-process reloads
- Team picker, agent picker, focus lens
- Existing RPC, session, protocol structures

## Changes

### 1. Run-focused monitor always loads team context

**File**: `internal/tui/monitor_bootstrap.go` – `newMonitorModel`

- Load run; require `run.Runtime.TeamID` (non-empty). If missing, treat as error or invalid state.
- Load team manifest for that teamID.
- Populate `m.teamID`, `m.teamRunIDs`, `m.teamRoleByRunID`, `m.teamCoordinatorRunID`, `m.teamCoordinatorRole`.
- `runID` remains the attached run (initial focus). Team context is always present.

**No fallback** for missing team. Session start already creates team; runs have `Runtime.TeamID`. If we hit a run without it, fail or fix the source.

### 2. Remove /team block

**File**: `internal/tui/monitor_commands.go` – `cmdTeam`

- Remove `if strings.TrimSpace(m.teamID) == ""` error.
- `/team` always opens the picker. Team context is always loaded (step 1).

### 3. Simplify Init and tick

**File**: `internal/tui/monitor.go`, `monitor_update.go`

- Remove branches that skip team loading when `teamID == ""`.
- Init: always load team manifest, team status, team events when we have a session.
- Tick: same. No "if teamID then load team stuff" – we always have teamID for active sessions.

### 4. Detached mode

- Detached = no session, no run, no team. `teamID == ""` is valid only when detached.
- `/team` when detached: show "no active context" or similar. Don't open picker with empty items.
- Keep detached as the one exception: no team because no session.

### 5. Remove standalone-specific code elsewhere

**Audit and remove**:

- `monitor_data.go`, `monitor_commands.go`, `monitor_tabs.go`, etc.: any `if teamID == ""` that implies "standalone mode" or "single-run mode" as a first-class path. Replace with: detached vs attached. Attached = always has team.
- `monitor_session_picker.go`: mode check – use `"multi-agent"` (or drop mode check for team routing; teamID on session is the source of truth).
- `MonitorSwitchTeamError` / `MonitorSwitchRunError`: audit. If unused, remove. If used for restart flow, keep but rename for clarity ("reload" not "switch").

### 6. Session picker: always prefer team

**File**: `internal/tui/monitor_session_picker.go`

- When selecting a session: if session has `TeamID`, send `monitorSwitchTeamMsg`. Always.
- No `monitorSwitchRunMsg` for sessions that have a team – use team view. User can focus a run via `/team` if desired.

### 7. Tests

- Remove `TestMonitorHandleCommand_TeamCommandOnlyInTeamMode` (tests the block we're removing).
- Add: run-focused monitor loads team context; `/team` opens picker with agents.
- Update any test that asserts "teamID empty" for non-detached scenarios.

## Implementation Order

1. **newMonitorModel**: always load team from `run.Runtime.TeamID`; require it for non-detached.
2. **cmdTeam**: remove block.
3. **Init/tick**: remove `teamID == ""` branches for active sessions.
4. **Detached**: keep as only exception; `/team` when detached = no-op or message.
5. **Audit**: grep for `teamID == ""`, `teamID != ""`; remove or simplify each.
6. **Session picker**: always use team when available.
7. **MonitorSwitchTeamError** etc.: audit and cleanup.
8. **Tests**: update/remove.

## Mode Display (single-agent vs multi-agent)

Keep `Mode` for display (session picker, etc.) as "single-agent" or "multi-agent" based on `len(roles)`. This is purely for display. No logic should branch on it – only on team structure (teamID, teamRunIDs, etc.).

## Summary

| Before | After |
|--------|-------|
| Run-focused: no team, /team blocked | Run-focused: always load team; /team works |
| Branches on teamID empty | Team always present when attached |
| Standalone vs team as distinct modes | One model: team (1 or N agents) |
| Legacy/fallback handling | None; require team |
