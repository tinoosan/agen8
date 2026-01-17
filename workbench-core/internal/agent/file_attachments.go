package agent

// FileAttachment is a bounded file snapshot attached to a single user turn.
//
// Attachments are a host-side convenience for general agent workflows:
//   - the user can reference files with @path syntax in the chat input
//   - the host resolves those references against /workdir (and other sources)
//   - the constructor injects the referenced file contents into the system prompt
//     so the model can reason and produce correct edits.
//
// Attachments are not persisted as primary records (history/events are). The
// host may choose to record a manifest describing what was attached.
type FileAttachment struct {
	// Token is the original @token from the user message (without the "@").
	Token string

	// VPath is the resolved VFS path for the attachment (e.g. "/workdir/go.mod").
	VPath string

	// DisplayName is a human-friendly label (usually a workdir-relative path).
	DisplayName string

	// Content is the included bytes as a UTF-8 string. It may be truncated.
	Content string

	BytesTotal    int
	BytesIncluded int
	Truncated     bool
}
