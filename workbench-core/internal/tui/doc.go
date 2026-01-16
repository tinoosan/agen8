// Package tui provides the interactive terminal UI for Workbench.
//
// Workbench's TUI is intentionally dumb: it is a renderer and input controller
// for the existing agent loop and event system.
//
// Design goals:
//   - One integrated scrollback timeline:
//     user messages, host events, and agent responses are rendered together.
//   - Streaming observability:
//     events emitted by the host are rendered as they occur (no separate log pane).
//   - Reliable multiline input:
//   - default: single-line; Enter sends
//   - multiline mode: Enter inserts newline; Ctrl+Enter sends
//   - multiline mode can be toggled, and can be auto-enabled when pasted text contains newlines.
//
// Workbench stores the authoritative provenance record on disk (trace/history).
// The TUI is a presentation layer over those same events.
package tui
