// Package vfs provides the Virtual File System (VFS) abstraction for Workbench.
//
// The agent interacts with files, results, tools, and artifacts through this mount
// table. `FS` manages mount points such as `/project`, `/scratch`, `/results`, `/tools`,
// `/history`, `/log`, and `/skills`. Resources implement `Reader`, `Writer`, and
// `Lister` semantics so the runtime can attach real directories, synthetic logs, or
// memory-backed buffers without leaking implementation details.
//
// # Key Concepts
//
//   - FS: Manages mounts + path resolution. Consumers call `Resolve`, `List`, `Read`, etc.
//   - Mount Point: Named location (`vfs.MountProject`, `vfs.MountSkills`, etc.) that maps
//     to a `Resource` implementation.
//   - Resource: Implements file operations for its mount (`DirResource`, `TraceResource`,
//     `HistoryResource`, etc.). Resources can enforce permissions, apply caching, or
//     synthesize data for the agent.
//
// # Usage Pattern
//
// Runtime initialization creates an `FS`, mounts the configured paths, and exposes
// an `agent.HostExecutor` that forwards host operations to these mounts. Hosts can
// also add their own mounts (e.g., `/plan` or `/secrets`) by registering resources
// before handing the FS to the agent. Keep in mind that VFS paths are always absolute
// (`/foo/bar`) and that `/tools` is reserved for tool discovery; hosts should not
// list inside tool directories, only read the manifest via `/tools/<toolId>`.
package vfs
