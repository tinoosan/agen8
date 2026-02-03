package ports

// ResultWriter persists tool call outputs and artifacts.
type ResultWriter interface {
	PutCall(callID string, responseJSON []byte) error
	PutArtifact(callID, artifactPath, mediaType string, content []byte) error
}
