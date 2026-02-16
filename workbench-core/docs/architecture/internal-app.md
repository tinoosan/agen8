# Package: internal/app

## Purpose
The `app` package is the operational brain of the workbench. it provides the daemon implementation, RPC server, and session management logic. It is responsible for bootstrapping the entire system, initializing storage, managing the runtime supervisor for active runs, and serving as the interface for both the Terminal User Interface (TUI) and headless background workers.

## Exported Types/Functions
- `RunDaemon`: Starts the autonomous worker loop for polling and executing tasks.
- `RuntimeSupervisor`: Manages the lifecycle and resource isolation of active agent runtimes.
- `RpcServer`: Implements the JSON-RPC interface for remote control and monitoring.
- `RpcSession`: Handles session-specific RPC requests (chat, tool calls, events).
- `Daemon`: The central struct coordinating the supervisor, RPC server, and background workers.

## Package Dependencies
```mermaid
graph TD
    internal_app["internal/app"] --> pkg_runtime["pkg/runtime"]
    internal_app --> pkg_session["pkg/agent/session"]
    internal_app --> pkg_store["pkg/store"]
    internal_app --> internal_store["internal/store"]
    internal_app --> pkg_protocol["pkg/protocol"]
```

## Daemon Bootstrapping Sequence
```mermaid
sequenceDiagram
    participant Main as main
    participant Daemon as app.Daemon
    participant Super as app.RuntimeSupervisor
    participant RPC as app.RpcServer

    Main->>Daemon: Initialize with Config
    Daemon->>Super: Start Runtime Supervisor
    Daemon->>RPC: Start RPC Server
    Daemon->>Daemon: Start Host Operation Handlers
    Daemon->>Daemon: Start Background Token Monitoring
    Main->>Daemon: Block on Context/Signal
```

## Runtime Flow: RPC Session Chat
```mermaid
sequenceDiagram
    participant UI as TUI/Client
    participant RPC as app.RpcSession
    participant Session as session.Session
    participant Agent as pkg/agent

    UI->>RPC: ChatRequest(goal)
    RPC->>Session: NewSession(goal)
    Session->>Agent: Run(goal)
    loop Agent Loop
        Agent->>RPC: Emit Progress Event
        RPC-->>UI: RPC Notification (Event)
    end
    Agent-->>RPC: Return RunResult
    RPC-->>UI: ChatResponse(result)
```

## Invariants
- The `RuntimeSupervisor` is the single source of truth for all active runtimes; it must prevent resource leaks by ensuring proper shutdown of unused environments.
- RPC sessions must be isolated; a client should only be able to interact with runs/tasks they are authorized for (context-based).
- The daemon must be resilient to LLM availability; it should implement retries and graceful degradation where possible.
- Application-level bootstrapping (seeding defaults, initializing store) must compete before the RPC server becomes available.
