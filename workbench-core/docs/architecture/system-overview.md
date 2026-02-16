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
        Session["session.Session (Manager)"]
        Agent["agent.DefaultAgent (Loop)"]
        Runtime["runtime.Runtime (Factory)"]
    end

    subgraph PersistenceLayer [Persistence Layer]
        Store["pkg/store (DailyMemoryStore)"]
        TaskStore["pkg/agent/state (TaskStore)"]
    end

    TUI --> RPC
    RPC --> Daemon
    Daemon --> Supervisor
    Supervisor --> Runtime
    Daemon --> Session
    Session --> Agent
    Agent --> Runtime
    Session --> TaskStore
    Agent --> Store
```

## Global Task State Machine

The workbench transitions tasks through the following lifecycle, specifically handling the complexity of recursive agent spawning (delegation).

```mermaid
stateDiagram-v2
    [*] --> pending: Task created (inbox or CLI)
    pending --> active: Session picks up task
    active --> delegated: agent_spawn tool called
    delegated --> pending: Child final_answer -> callback processed
    active --> succeeded: final_answer received
    active --> failed: Error or run timeout
    succeeded --> [*]
    failed --> [*]
```

## Recursive Delegation Flow

The following sequence illustrates the flow from a user's initial request through a subagent delegation and final resolution.

```mermaid
sequenceDiagram
    participant User as User (TUI)
    participant Parent as Parent Agent (Session)
    participant Store as Task Store / DB
    participant Child as Subagent (Session)

    User->>Parent: "Implement feature X"
    Parent->>Parent: Thinking...
    Parent->>Store: agent_spawn(subtask: "Design X")
    Store-->>Parent: ChildRunID Created
    Parent->>User: Update (Task status: Delegated)
    
    Note over Child: Subagent starts
    Child->>Store: Create Callback when done
    
    Parent->>Store: Heartbeat polling...
    Store-->>Parent: Callback found for ChildRunID
    Parent->>Parent: Transition status: Pending
    
    Parent->>Parent: Resume thinking with Child output
    Parent->>User: final_answer("Feature X implemented...")
```

## System-Wide Invariants
1.  **Isolation**: Every agent run must operate within a unique `Runtime` with its own VFS and resource limits.
2.  **Statelessness**: The `agent.Loop` should remain stateless; all persistence and memory must be externalized to the `Store` or `TaskStore`.
3.  **Traceability**: Every host operation must emit an event that can be traced back to a specific `RunID` and `SessionID`.
4.  **Consistency**: Task state transitions are governed by the `session` and must be atomic within the underlying `TaskStore`.
