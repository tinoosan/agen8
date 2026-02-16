# System Architecture Overview

This document provides a high-level view of the Workbench architectural components and their interactions during the lifecycle of a task.

## Component Map

```mermaid
graph TB
    subgraph ClientLayer [Client Layer]
        TUI["Terminal UI"]
        Terminal["Remote Terminal/CLI"]
    end

    subgraph DaemonLayer [Daemon Layer]
        Daemon["app.Daemon (Orchestrator)"]
        RPC["app.RpcServer (JSON-RPC)"]
        Supervisor["app.RuntimeSupervisor"]
    end

    subgraph CoreLayer [Core Logic Layer]
        SessionSvc["pkg/services/session (Service)"]
        Agent["agent.DefaultAgent (Loop)"]
        Runtime["runtime.Runtime (Factory)"]
    end

    subgraph PersistenceLayer [Persistence Layer]
        SessionStore["Session Store (SQLite/Memory)"]
        Store["pkg/store (DailyMemoryStore)"]
        TaskStore["pkg/agent/state (TaskStore)"]
    end

    TUI --> RPC
    RPC --> Daemon
    Daemon --> Supervisor
    Supervisor --> Runtime
    Daemon --> SessionSvc
    RPC --> SessionSvc
    SessionSvc --> SessionStore
    SessionSvc --> Supervisor
    SessionSvc --> Agent
    Agent --> Runtime
    SessionSvc --> TaskStore
    Agent --> Store
```

**Session Service** (`pkg/services/session`): The daemon and RPC access all session and run data only through this service. It is implemented by the **Manager**, which uses a Store (SQLite or in-memory) and the RuntimeSupervisor for stop/delete. See [pkg-services-session](pkg-services-session.md) for the full API and diagrams.

## Global Task State Machine

The workbench transitions tasks through the following lifecycle. Coordination tasks complete in Succeeded/Failed/Canceled; callbacks are separate tasks.

```mermaid
stateDiagram-v2
    [*] --> pending: Task created (inbox or CLI)
    pending --> active: Session picks up task
    active --> succeeded: final_answer or spawn (task completed)
    active --> failed: Error or run timeout
    succeeded --> [*]
    failed --> [*]
```

## Subagent Spawn and Callback Flow

The following sequence illustrates the flow from a user's initial request through a subagent spawn and callback processing.

```mermaid
sequenceDiagram
    participant User as User (TUI)
    participant Parent as Parent Agent (Session)
    participant Store as Task Store / DB
    participant Child as Subagent (Session)

    User->>Parent: "Implement feature X"
    Parent->>Parent: Thinking...
    Parent->>Store: task_create(spawn_worker: "Design X")
    Store-->>Parent: ChildRunID created
    Parent->>Store: CompleteTask (Succeeded, summary)
    Parent->>User: Task completed (coordination done)

    Note over Child: Subagent runs child task
    Child->>Store: Create Callback when done

    Parent->>Store: Picks up callback as normal task
    Parent->>Parent: Run(callback task), task_review, etc.
    Parent->>User: Callback task completed
```

## System-Wide Invariants

1.  **Isolation**: Every agent run must operate within a unique `Runtime` with its own VFS and resource limits.
2.  **Statelessness**: The `agent.Loop` should remain stateless; all persistence and memory must be externalized to the `Store` or `TaskStore`.
3.  **Traceability**: Every host operation must emit an event that can be traced back to a specific `RunID` and `SessionID`.
4.  **Consistency**: Task state transitions are governed by the `session` and must be atomic within the underlying `TaskStore`.
