// Package store defines the persistent stores used by the runtime and agent.
//
// These interfaces decouple runtime components from specific databases or file
// layouts so hosts may provide custom storage implementations (SQLite, Redis, in-memory, etc.).
//
// # Key Responsibilities
//
//   - `HistoryStore`: Appends to the immutable history log and loads recent op pairs.
//   - `DailyMemoryStore`: Captures shared memory files for later context injection.
//   - `TraceStore`: Persists reasoning traces for debugging or analysis.
//   - `ConstructorStateStore`: Holds per-run constructor state + manifest blobs so agents can resume safely.
//
// # Usage Pattern
//
// Hosts implement the interfaces they need and pass them into `runtime.BuildConfig`. The
// runtime wires them into `resources.Factory`, which then exposes read-only VFS resources
// (`/history`, `/log`, `/memory`) and persistence hooks. Tests can use the provided
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
//   - `DailyMemoryStore`: reads and writes memory files under the shared /memory mount.
//   - `TraceStore`: captures reasoning traces for debugging, diagnostics, and analysis.
//   - `ConstructorStateStore`: holds per-run constructor state + manifest blobs so agents can resume safely.
package store
