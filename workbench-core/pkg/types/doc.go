// Package types defines Workbench's core data model types and host/agent protocols.
//
// It documents the contracts for host primitives (fs_*, shell_exec, http_fetch, trace_run)
// plus the event results that the agent and host exchange.
//
// # Host Operation Protocol
//
// Hosts and agents communicate through `types.HostOpRequest`/`types.HostOpResponse` via
// `agent.HostExecutor`. Each `Op` (e.g., `fs_read`, `shell_exec`, `trace_run`)
// has specific validation rules declared in `HostOpRequest.Validate()`, and the request
// normalization in `normalizeHostOp` keeps casing + aliasing consistent. All host primitives
// expect absolute VFS paths for file ops and required payloads (text, input JSON, etc.) before they run.
//
// # Events, History, and Stability
//
// Events (see `package events`) use `events.Event` as the runtime emission payload for logs,
// tool usage, and telemetry. When persisted, those payloads are recorded as `types.EventRecord`
// (event id, run id, timestamp, type, message, and optional data; see EventRecord) by the host-side event store.
//
// The `events.MultiSink` abstraction allows hosts to fan-out events, and `events.Emitter`
// enforces that a run ID + sink must exist before emitting.
//
// The core types in this package are intended to remain stable within a major release because
// they define the low-level host/agent protocol. Any change that would break these structs should
// be guarded by a clear migration or version bump.
package types
