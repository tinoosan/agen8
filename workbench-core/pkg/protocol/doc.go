// Package protocol defines the Workbench App Server protocol.
//
// The protocol models interactive work as:
//   - Thread: a durable session container (maps to workbench types.Session).
//   - Turn: a single user -> agent execution cycle within a thread.
//   - Item: an atomic unit of work within a turn (messages, tool calls, reasoning).
//
// Transport is JSON-RPC 2.0 framed messages. A client sends requests to create/get
// threads and turns, and receives server notifications as turns and items progress.
//
// This package is types-only: it defines the JSON schema and constants used on the
// wire without importing internal packages or changing existing behavior.
package protocol
