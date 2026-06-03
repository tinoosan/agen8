// Package types defines Agen8's canonical domain model plus the host/agent operation protocol.
//
// Ownership rules:
//   - Canonical business entities live here: Task, Run, SpaceRuntime, EventRecord,
//     MemberMessage, and the rest of the runtime's durable domain model.
//   - Host operation request/response types also live here because they are the stable
//     contract between the agent runtime and its host executor.
//   - JSON-RPC framing, method constants, transport errors, and RPC params/results do not
//     belong here; those live in package protocol.
//
// Package protocol may still contain transitional domain-shaped types while the domain model
// cleanup is in progress. That existing debt is not precedent for new work: new canonical
// entities should be added in package types first and then referenced from transport layers.
//
// # Host Operation Protocol
//
// Hosts and agents communicate through `types.HostOpRequest`/`types.HostOpResponse` via
// `agent.HostExecutor`. Each `Op` (e.g., `read_file`, `shell_exec`, `browser`)
// has specific validation rules declared in `HostOpRequest.Validate()`, and the request
// normalization in `normalizeHostOp` keeps casing + aliasing consistent. All host primitives
// expect absolute VFS paths for file ops and required payloads (text, input JSON, etc.) before they run.
//
// # Events, History, and Stability
//
// Events use `types.EventRecord` as the canonical emission and storage payload for logs,
// tool usage, and telemetry. The events domain service lives in `pkg/services/events/`
// (domain/, app/, infra/, rpc/ layers). The app layer's `Emitter` and `MultiSink`
// abstraction allows hosts to fan-out events with a run ID + sink.
//
// The core types in this package are intended to remain stable within a major release because
// they define the low-level host/agent protocol. Any change that would break these structs should
// be guarded by a clear migration or version bump.
package types
