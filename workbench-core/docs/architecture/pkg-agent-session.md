# Package: pkg/agent/session

## Purpose
The `session` package is the orchestrator for task execution and lifecycle management. It manages an "inbox" of tasks, handling their progression from discovery to completion. It bridges the gap between the stateless `agent` loop and the persistent `store`, ensuring that subagent delegations are correctly tracked and resumed through heartbeats and callback processing.

## Exported Types/Functions
- `Session`: The main coordinator struct for task execution.
- `New(cfg Config)`: Initializes a new session with agent, store, and profile configuration.
- `Session.Run(ctx context.Context)`: The entry point for processing the session's task inbox.
- `Session.SetPaused(paused bool)`: Controls the execution state of the session.
- `Config`: Configuration struct for session behavior, including delegation roles.

## Package Dependencies
```mermaid
graph TD
    pkg_session["pkg/agent/session"] --> pkg_agent["pkg/agent"]
    pkg_session --> pkg_store["pkg/store"]
    pkg_session --> pkg_profile["pkg/profile"]
    pkg_session --> pkg_events["pkg/events"]
    pkg_session --> pkg_state["pkg/agent/state"]
```

## Task State Machine
```mermaid
stateDiagram-v2
    [*] --> Pending: Task discovered in inbox
    Pending --> Active: runTask begins
    Active --> Succeeded: final_answer received
    Active --> Failed: Error or run timeout
    Active --> Delegated: Subagent spawned (agent_spawn)
    Delegated --> Pending: All callbacks processed
    Succeeded --> [*]
    Failed --> [*]
```

## Runtime Flow: Subagent Delegation
```mermaid
sequenceDiagram
    participant Session as session.Session
    participant Agent as pkg/agent
    participant Store as pkg/store
    participant SubAgent as Subagent Session

    Session->>Agent: Run(task)
    Agent->>Store: task_create (Child Task)
    Agent-->>Session: Return (Delegated=true)
    Session->>Session: Mark task as Delegated
    
    Note over SubAgent: Subagent executes Child Task
    SubAgent->>Store: Create Callback
    
    Session->>Session: Heartbeat detects Callback
    Session->>Session: maybeResumeDelegatedTask
    Session->>Session: Transition Parent to Pending
    Session->>Agent: Run(task) with Resume Context
```

## Invariants
- A task in the `Delegated` state must not be processed by the parent agent until all its child callbacks are resolved.
- Session heartbeats are the primary mechanism for detecting changes in the external task store (e.g., child completions).
- The session must ensure that the VFS state and LLM history are correctly persisted to allow for task resumption.
