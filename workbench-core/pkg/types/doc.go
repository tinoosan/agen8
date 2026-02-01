// Package types defines Workbench's core data model types and host/agent protocols.
//
// It documents the contracts for host primitives (fs.*, tool.*, shell_exec, http_fetch, trace)
// plus the tool and event results that the agent and host exchange.
//
// # Host Operation Protocol
//
// Hosts and agents communicate through `types.HostOpRequest`/`types.HostOpResponse` via
// `agent.HostExecutor`. Each `Op` (e.g., `fs.read`, `tool.run`, `shell_exec`, `trace.events.latest`)
// has specific validation rules declared in `HostOpRequest.Validate()`, and the request
// normalization in `normalizeHostOp` keeps casing + aliasing consistent. All host primitives
// expect absolute VFS paths for file ops, timeout bounds for tool invocations, and required
// payloads (text, input JSON, etc.) before they run.
//
// # Tool Data Flow
//
// Tools are described with `tools.ToolManifest`/`tools.ToolAction`, and their results are
// captured in `types.ToolResponse`. Tool calls are orchestrated by the runtime's `tools.Orchestrator`,
// which persists `ToolResponse`/artifacts under `/results/<callId>` so later steps or
// host-side tooling can inspect what happened. The documents under `/tools` and `/results`
// form the public API surface for tool discovery, invocation, and auditing.
//
// # Events, History, and Stability
//
// Events (see `package events`) use `events.Event` as the runtime emission payload for logs,
// tool usage, and telemetry. When persisted, those payloads are recorded as `types.EventRecord`
// (event id + timestamp) by the host-side event store.
//
// The `events.MultiSink` abstraction allows hosts to fan-out events, and `events.Emitter`
// enforces that a run ID + sink must exist before emitting.
//
// The core types in this package are intended to remain stable within a major release because
// they define the low-level host/agent protocol. Any change that would break these structs should
// be guarded by a clear migration or version bump.
package types
