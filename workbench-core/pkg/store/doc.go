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
//   - `ProfileStore` / `ProfileCommitter`: Stores profiling events emitted by the agent.
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
// These interfaces are considered stable because they capture the persistence contracts the agent
// depends on. Adding new stores should generally not remove existing interfaces, and any
// extensions should aim for backward compatibility to avoid forcing a migration for hosts.
package store
