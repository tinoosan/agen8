// Package store defines the persistent stores used by the runtime and agent.
//
// These interfaces decouple runtime components from specific databases or file
// layouts so hosts may provide custom storage implementations (SQLite, Redis, in-memory, etc.).
//
// # Key Responsibilities
//
//   - `HistoryStore`: Appends to the immutable history log and loads recent op pairs.
//   - `ResultsStore` / `ResultsView`: Persists tool call metadata, responses, and artifacts.
//   - `MemoryStore` / `MemoryCommitter`: Captures agent memory commits for later context injection.
//   - `TraceStore`: Persists reasoning traces for debugging or analysis.
//   - `ConstructorStateStore`: Holds per-run constructor state + manifest blobs so agents can resume safely.
//
// # Usage Pattern
//
// Hosts implement the interfaces they need and pass them into `runtime.BuildConfig`. The
// runtime wires them into `resources.Factory`, which then exposes read-only VFS resources
// (`/results`, `/history`, `/trace`) and persistence hooks. Tests can use the provided
// in-memory helpers or mocks while production hosts rely on durable backends.
//
// # Stability
//
// These interfaces capture the persistence contracts the agent depends on.

// # Key Interfaces and Tokens
//
//   - `HistoryCursor`: opaque token returned by history readers that encodes a store-specific
//     read position (byte offset for disk, sequence number for SQLite). Hosts must treat it as
//     opaque and consume it with the same implementation that produced it.
//   - `HistoryStore`: journaling interface that appends events and reads them back via `LinesSince`
//     and `LinesLatest`.
//   - `ResultsStore` / `ResultsView`: structured storage for tool-call outputs and artifacts.
//   - `MemoryStore` / `MemoryCommitter`: captures and replays agent memory commits during a run.
//   - `TraceStore`: captures reasoning traces for debugging, diagnostics, and analysis.
//   - `ConstructorStateStore`: holds per-run constructor state + manifest blobs so agents can resume safely.
package store
