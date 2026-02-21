// Package events provides the event system for the agen8.
//
// It defines the core `Event` structure used for observability, history tracking,
// and communication between components.
//
// # Key Concepts
//
//   - Event: A structured record of an occurrence (e.g., tool call, log message, error).
//   - Sinks: Destinations for events (e.g., console, history file, debug log).
//   - History: A persistent log of events that allows the agent to recall past actions.
//
// # Usage Pattern
//
// Hosts construct `Events.Sink` implementations (e.g., console sink, history sink,
// store sink) and combine them via `events.MultiSink`. Emitters require a `runID`
// and a non-nil sink before calling `Emit`, ensuring runtime metadata is attached
// to every event. When multiple sinks are needed, `MultiSink` fans them out while
// short-circuiting if any sink fails; hosts should manage failures (retry, log, etc.)
// depending on their tolerance for missing telemetry.
//
// # Observability Guidance
//
// Events are emitted from runtime components during tool invocation, history
// recording, and debug logging. Because events may carry `StoreData`, `Console`,
// or `History` flags, consumers should implement sinks that honor those hints.
// This package is a stable foundation for logging + observability, so changes to the
// `Event` schema should be treated as breaking and accompanied by release notes.
//
// # API Surfaces
//
//   - `Event`: carries the runtime type, message, metadata maps, and delivery hints
//     that sinks use to decide whether to write to console, history, or other stores.
//   - `Sink`: consumes `Event` payloads wrapped in an `events.Message` (run ID + payload); hosts ship `Sink`
//     implementations such as console, history, or store sinks.
//   - `MultiSink`: fans events out to multiple sinks while collecting partial errors.
//   - `Emitter`: wraps a run ID + sink pair so much of the runtime can emit without
//     managing the sink lifecycle.
package events
