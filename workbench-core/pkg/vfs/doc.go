// Package vfs provides the Virtual File System (VFS) abstraction.
//
// It allows the workbench to present a unified view of various resources (local files,
// memory buffers, synthetic logs) to the agent.
//
// # Key Concepts
//
//   - FS: The main file system object that manages mount points.
//   - Mount Point: A location in the VFS tree (e.g., `/project`) mapped to a resource.
//   - Resource: A backend that implements file operations (Read, Write, List) for a mount.
//
// This abstraction isolates the agent from the host OS and allows for safe, controlled
// access to files.
package vfs
