package protocol

import "github.com/tinoosan/agen8-mcp-server/pkg/types"

// ArtifactListResult is the result for artifact.list.
type ArtifactListResult struct {
	Nodes []types.ArtifactNode `json:"nodes"`
}

// ArtifactSearchResult is the result for artifact.search.
type ArtifactSearchResult struct {
	Nodes      []types.ArtifactNode `json:"nodes"`
	MatchCount int                  `json:"matchCount"`
}

// ArtifactGetResult is the result for artifact.get.
type ArtifactGetResult struct {
	Artifact        types.ArtifactNode `json:"artifact"`
	Content         string             `json:"content"`
	ContentKind     string             `json:"contentKind,omitempty"`
	ContentType     string             `json:"contentType,omitempty"`
	ContentEncoding string             `json:"contentEncoding,omitempty"`
	BytesB64        string             `json:"bytesB64,omitempty"`
	Truncated       bool               `json:"truncated"`
	BytesRead       int                `json:"bytesRead"`
	FileSize        int64              `json:"fileSize,omitempty"`
}
