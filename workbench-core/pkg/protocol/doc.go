// Package protocol defines the Workbench App Server protocol.
//
// The protocol models interactive work as:
//   - Thread: a durable session container (maps to workbench types.Session).
//   - Turn: a single user -> agent execution cycle within a thread.
//   - Item: an atomic unit of work within a turn (messages, tool calls, reasoning).
//   - Artifact: indexed deliverable files browsed/searched via artifact.* methods.
//
// Transport is JSON-RPC 2.0 framed messages. A client sends requests to create/get
// threads and turns, and receives server notifications as turns and items progress.
//
// In addition to wire types + constants, this package provides an adapter sink
// (see EventSink) that maps host events (`types.EventRecord`) into protocol
// notifications.
package protocol
