package types

// AttachmentRef references an attachment associated with a message.
type AttachmentRef struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	URI       string `json:"uri,omitempty"`
}

// ArtifactRef references a generated artifact.
type ArtifactRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	URI  string `json:"uri,omitempty"`
}
