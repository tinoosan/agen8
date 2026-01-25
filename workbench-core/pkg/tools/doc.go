// Package tools implements the tool management system.
//
// It provides the infrastructure for discovering, registering, and executing tools.
// This includes both "builtin" host operations (like shell execution) and external
// tools defined by manifests or plugins.
//
// # Key Components
//
//   - Registry: A collection of available tools and their definitions.
//   - Manifest: The schema defining a tool's inputs, outputs, and capabilities.
//   - Runner: The component responsible for actually executing a tool call.
package tools
