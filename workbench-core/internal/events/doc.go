// Package events provides an event emission and routing system for workbench.
//
// Events are structured records of actions, state changes, and outputs that occur
// during a workbench run. The events package uses a sink pattern to route events
// to multiple destinations without coupling event producers to consumers.
//
// # Architecture
//
// The event system has three main components:
//
//  1. Emitter: Produces events and dispatches them to registered sinks
//  2. Sink: Receives events and processes them (console output, persistence, etc.)
//  3. Event: Structured data describing what happened
//
// # Available Sinks
//
// The package provides three built-in sink implementations:
//
//   - ConsoleSink: Logs events to stdout for real-time monitoring
//   - HistorySink: Persists events to a JSONL history file for provenance
//   - StoreSink: Mirrors events to the trace resource for agent polling
//
// # Usage Pattern
//
//	emitter := events.NewEmitter()
//	emitter.AddSink(events.NewConsoleSink())
//	emitter.AddSink(events.NewHistorySink(runId))
//	emitter.Emit(types.Event{Type: "action_start", Message: "Running tool..."})
//
// # Event Flow
//
// Events flow from producers (agent loop, tool runner) through the emitter to
// multiple sinks in parallel. Each sink processes events independently, so a
// failure in one sink (e.g., write error) does not affect other sinks.
package events
