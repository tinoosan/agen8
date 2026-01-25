package agent

// FileAttachment is a bounded file snapshot attached to a single user turn.
type FileAttachment struct {
	// Token is the original @token from the user message (without the "@").
	Token string

	// VPath is the resolved VFS path for the attachment (e.g. "/project/go.mod").
	VPath string

	// DisplayName is a human-friendly label (usually a workdir-relative path).
	DisplayName string

	// Content is the included bytes as a UTF-8 string. It may be truncated.
	Content string

	BytesTotal    int
	BytesIncluded int
	Truncated     bool
}
