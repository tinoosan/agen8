package ports

import "context"

// ResultWriter persists tool call outputs and artifacts.
type ResultWriter interface {
	PutCall(callID string, responseJSON []byte) error
	PutArtifact(callID, artifactPath, mediaType string, content []byte) error
}

// HistoryAppender appends one history line to a history store.
type HistoryAppender interface {
	AppendLine(ctx context.Context, line []byte) error
}
