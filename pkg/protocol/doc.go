// Package protocol defines Agen8's JSON-RPC transport surface.
//
// Scope rules:
//   - JSON-RPC framing, transport errors, method constants, notifications, and RPC
//     param/result structs belong here.
//   - Canonical business entities do not belong here; those belong in package types.
//   - When a transport result needs to expose domain data, it should reference
//     package types instead of redefining the canonical entity in protocol.
//   - JSON-RPC numeric error codes belong here; semantic/internal error codes do not.
//
// The package still contains some historical domain-shaped types from before the current
// layer boundary was formalized. Those are transitional and are being moved out in follow-up
// issues. New domain entities should not be introduced here.
//
// Transport is JSON-RPC 2.0 framed messages. A client sends requests, receives results,
// and subscribes to protocol notifications as runtime state changes.
package protocol
