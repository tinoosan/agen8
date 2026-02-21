// Package tui provides the interactive terminal UI for Agen8.
//
// Agen8's TUI is intentionally dumb: it is a renderer and input controller
// for the existing agent loop and event system.
//
// Design goals:
//   - One integrated scrollback timeline:
//     user messages, host events, and agent responses are rendered together.
//   - Streaming observability:
//     events emitted by the host are rendered as they occur (no separate log pane).
//   - Reliable multiline input:
//   - default: single-line; Enter sends
//   - multiline mode: Enter inserts newline; Ctrl+Enter (or Ctrl+S) sends
//   - multiline mode can be toggled (Ctrl+G), and can be auto-enabled when pasted text contains newlines.
//
// Agen8 stores the authoritative provenance record on disk (trace/history).
// The TUI is a presentation layer over those same events.
package tui
