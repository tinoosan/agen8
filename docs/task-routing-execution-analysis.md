# Agen8 Task Routing and Execution Analysis

Date: 2026-03-05  
Scope: Current repository behavior (task creation, routing, assignment, execution, scheduling, message-bus coupling)

## Short Answer

Agen8 currently behaves as a **hybrid model (C)**:

- **Agent-assigned routing** exists (`assigned_to_type = "agent"`, `assigned_to = <runID>`)
- **Role-based routing** exists (`assigned_to_type = "role"`, `assigned_to = <role>`)
- **Team-level routing** also exists (`assigned_to_type = "team"`, `assigned_to = <teamID>`, typically coordinator-consumed)

Runtime workers consume from a shared **message bus envelope** (`messages` table) using assignee-aware claim filters.

---

## 1. Task Creation

### Primary task type and routing fields

- `types.Task` in `pkg/types/task.go`
- Routing-identifying fields:
  - `SessionID`
  - `RunID`
  - `TeamID`
  - `AssignedRole`
  - `AssignedToType` (`team | role | agent`)
  - `AssignedTo`
  - `ClaimedByAgentID`
  - `RoleSnapshot`

### Creation entry points

1. **Tool-based creation (`task_create`)**
   - File: `pkg/agent/hosttools/task_create.go`
   - Struct: `TaskCreateTool`
   - Team mode sets role assignment (`AssignedToType="role"`, `AssignedTo=assignedRole`)
   - Spawn-worker path sets direct agent assignment (`AssignedToType="agent"`, `AssignedTo=childRunID`)

2. **RPC task creation (`task.create`)**
   - File: `internal/app/rpc_session.go`
   - Method: `(*RPCServer).taskCreate`
   - Resolves defaults to agent/role/team assignment based on scope and inputs.

3. **RPC turn creation (`turn.create`)**
   - File: `internal/app/rpc_session.go`
   - Method: `(*RPCServer).turnCreate`
   - Creates a task envelope for user input (`messageKind=user_input` metadata), usually agent-assigned.

4. **Webhook ingestion**
   - Files:
     - `internal/webhook/payload.go` (`BuildStandaloneTask`, `BuildTeamTask`)
     - `internal/webhook/ingester.go` (`WebhookTaskIngester.IngestTask`)

5. **Team bootstrap seed**
   - File: `pkg/services/team/service.go`
   - Function: `SeedCoordinatorTask`
   - Creates initial team task for coordinator role.

6. **System-generated tasks during runtime**
   - Heartbeats: `pkg/agent/session/session.go` (`handleHeartbeat`)
   - Callback/review tasks: `pkg/agent/session/session.go` (`maybeCreateCoordinatorCallback`)
   - Retry/escalation tasks: `pkg/services/task/manager.go` (`CreateRetryTask`, `CreateEscalationTask`)

### Persistence defaulting/canonicalization

- File: `pkg/agent/state/sqlite_task_store.go`
- Method: `(*SQLiteTaskStore).CreateTask`
- If `AssignedToType` is empty:
  - team + assigned role -> `role`
  - team without role -> `team`
  - no team -> `agent`

---

## 2. Task Assignment Model

Agen8 supports both assignment styles simultaneously.

- **Direct-to-agent/run assignment**
  - `AssignedToType="agent"`, `AssignedTo=<runID>`
  - Common in standalone, child/subagent callbacks, retries, and some RPC flows.

- **Role-based assignment**
  - `AssignedToType="role"`, `AssignedTo=<roleName>`
  - Common in team delegation, coordinator callbacks, role heartbeats.

- **Team-wide assignment**
  - `AssignedToType="team"`, `AssignedTo=<teamID>`
  - Coordinator sessions include this claim filter.

Normalization + validation is performed by:

- `pkg/services/task/oracle.go`
- Struct: `RoutingOracle`
- Methods: `NormalizeCreate`, `NormalizeUpdate`, `ValidateCompletion`, internal `normalize`

---

## 3. Role <-> Run Mapping

### Mapping source of truth

- Team manifest types:
  - `pkg/services/team/types.go`
  - `Manifest` (fields include `CoordinatorRole`, `CoordinatorRun`, `Roles []RoleRecord`)
  - `RoleRecord` (`RoleName`, `RunID`, `SessionID`)

### How mapping is established

- `internal/app/session_start_service.go` (`sessionStart`)
  - Creates one run per role from profile
  - Writes `RoleRecord` entries into manifest
  - Sets coordinator run ID

### How mapping is used at runtime

- `internal/app/rpc_team.go`: `loadTeamManifestRunRolesFromStore`
  - builds `roleByRun` map from manifest
- `internal/app/daemon_runtime_supervisor.go`: `syncOnce`
  - if team run has empty `run.Runtime.Role`, resolves it from manifest map

### Can multiple runs share same role?

- **Profile-defined startup path**: effectively one run per role (because role names are unique in profile validation: `ValidateTeamRoles` in `pkg/services/team/service.go`).
- **Data model/storage level**: no hard DB-level uniqueness shown on role name across runs; the manifest structure itself does not enforce a reverse uniqueness constraint. So duplicates are not structurally impossible.

---

## 4. Task Execution

### Worker that pulls and runs work

- Per-run worker session:
  - `pkg/agent/session/session.go`
  - `Session.Run` -> `drainInbox` -> `drainInboxMessages`

- Workers are spawned by runtime supervisor:
  - `internal/app/daemon_runtime_supervisor.go`
  - `spawnManagedRun` creates `agentsession.Session`
  - `startManagedWorkerLoop` executes `workerSession.Run(...)`

### Queue/polling mechanism

- Queue substrate: `messages` table in SQLite
  - `pkg/agent/state/sqlite_task_store.go` (table creation)
- Claim operation:
  - `(*SQLiteTaskStore).ClaimNextMessage`
  - SQL filters by `status=pending`, `visible_at`, optional assignee routing via `EXISTS` join to `tasks`
  - ordering: `ORDER BY priority ASC, created_at ASC`

- Session claim filters are built per worker in:
  - `Session.buildMessageClaimFilters`
  - Agent filter always present (`agent:<runID>`)
  - Team role filter added in team mode (`role:<roleName>`)
  - Team filter added for coordinator (`team:<teamID>`)

- Triggering mechanism is mixed:
  - wake-channel driven (`RequireWakeCh=true` in supervisor wiring)
  - polling/backoff timer fallback in session loop

---

## 5. Scheduler Behavior (multiple runs for same role)

If multiple workers are eligible for the same assignee target (for example same role):

- They contend on `ClaimNextMessage` for the same role-filtered queue view.
- Selection is effectively **first successful claimer** under transactional update.
- Not fixed to a pre-bound run, and not explicitly deterministic by run identity.
- Determinism only applies to message ordering candidates (`priority`, `created_at`), not which competing worker wins the claim.

So the worker choice is best described as:

- **Queue ordering deterministic per message candidates**
- **Consumer selection race/availability based**

---

## 6. Message Bus Relationship

Tasks are explicitly coupled to a message bus envelope.

### Core types

- `types.AgentMessage` in `pkg/types/agent_message.go`
  - Includes `TaskRef` and optional embedded `Task`
- `state.MessageStore` in `pkg/agent/state/store.go`
  - Publish/claim/ack/nack/requeue APIs
- `state.MessageClaimFilter` in `pkg/agent/state/store.go`
  - Includes assignee filters (`AssignedToType`, `AssignedTo`)

### Persistence

- `tasks` table + `messages` table in `pkg/agent/state/sqlite_task_store.go`

### Critical code path

1. Task is created via `task.Manager.CreateTask` (`pkg/services/task/manager.go`)
2. Manager publishes corresponding inbox message envelope via `publishTaskMessage`
3. Worker session claims message (`ClaimNextMessage`) using role/agent/team filters
4. Worker claims corresponding task lease, runs task, completes task
5. Message is acked/nacked; terminal task completion also syncs terminal message state (`syncTaskMessagesTerminal`)

---

## End-to-End Flow (Creation -> Execution)

1. A producer creates `types.Task` (tool, RPC, webhook, team seed, or system path).
2. `task.Manager.CreateTask` applies routing normalization (`RoutingOracle`) and persists task.
3. Manager publishes `types.AgentMessage` (`kind=task` or `user_input`) with `TaskRef`.
4. A run worker (`Session.Run`) drains inbox and claims next matching message for its assignee filters.
5. Worker claims task lease, sets `ClaimedByAgentID` and role snapshot, runs agent logic.
6. Worker completes task (`succeeded|failed|canceled`) and writes artifacts/summary.
7. Worker ack/nack updates message state; manager ensures terminal task messages are acked.
8. Callbacks/retries/escalations may generate follow-up tasks, repeating the same bus flow.

---

## Model Determination

Current behavior is **Hybrid (C)**:

- Agent-assigned tasks: yes
- Role-based task routing: yes
- Team-assigned coordinator catch-all: yes
- Unified execution transport: message-bus claim/ack pipeline over SQLite

