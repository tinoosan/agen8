// Package runtime manages the execution environment for the Workbench agent.
//
// It handles the initialization and orchestration of the virtual file system (VFS),
// tool registry, and host operations. The Runtime struct acts as the central
// hub, connecting the agent's logic with the underlying system resources and
// capabilities.
//
// # Key Responsibilities
//
//   - VFS Setup: Mounts resources like `/project` (user files), `/scratch` (temp), and `/log`.
//   - Tool Wiring: Discovers builtin tools and loads external tools from disk.
//   - Execution Environment: Provides the `HostExecutor` needed by the agent to run commands.
//   - Orchestration: Ties together the agent, tools, and filesystem into a cohesive runtime.
package runtime
