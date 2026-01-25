// Package events provides the event system for the workbench.
//
// It defines the core `Event` structure used for observability, history tracking,
// and communication between components.
//
// # Key Concepts
//
//   - Event: A structured record of an occurrence (e.g., tool call, log message, error).
//   - Sinks: Destinations for events (e.g., console, history file, debug log).
//   - History: A persistent log of events that allows the agent to recall past actions.
package events
