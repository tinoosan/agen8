// Package runtime manages the execution environment for the Workbench agent.
//
// It handles the initialization and orchestration of the virtual file system (VFS),
// tool registry, and host operations. The Runtime struct acts as the central
// hub, connecting the agent's logic with the underlying system resources and
// capabilities. Key responsibilities include:
//
//   - VFS Setup: Mounting project, scratch, and log directories.
//   - Tool Management: Discovering and registering builtin and user-defined tools.
//   - Execution: Providing the context and mechanisms for running tool calls.
package runtime
