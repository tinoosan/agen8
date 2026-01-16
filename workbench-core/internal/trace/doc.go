// Package trace provides disk-based event storage for workbench trace resources.
//
// This package implements the storage layer that backs the /trace VFS mount,
// allowing agents to poll for new events via offset-based reads.
//
// # Storage Format
//
// Events are persisted in JSONL (JSON Lines) format:
//
//	{"eventId":"event-1","type":"action_start","message":"Running bash..."}
//	{"eventId":"event-2","type":"action_result","message":"Command completed"}
//
// Each line is a complete JSON object representing one event. This format
// enables efficient append operations and byte-offset-based seeking.
//
// # Offset-Based Retrieval
//
// The trace system uses byte offsets to enable incremental polling:
//
//  1. Agent reads events: fs.Read("/trace/events.since/0")
//  2. System returns events and nextOffset (file size)
//  3. Agent stores nextOffset
//  4. Later: fs.Read("/trace/events.since/<nextOffset>")
//
// This pattern allows agents to efficiently poll for new events without
// re-reading the entire event log.
//
// # Disk Store Implementation
//
// The DiskStore manages:
//   - Append-only writes to events.jsonl
//   - Offset-based reads with size limits
//   - Thread-safe concurrent access
//
// # File Location
//
// Events are stored under:
//
//	data/runs/<runId>/trace/events.jsonl
package trace
