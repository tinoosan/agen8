package protocol

import "time"

// ArtifactNode is a tree-ready artifact row for browsing/searching deliverables.
type ArtifactNode struct {
	NodeKey   string `json:"nodeKey"`
	ParentKey string `json:"parentKey,omitempty"`
	Kind      string `json:"kind"` // day | role | stream | task | file
	Label     string `json:"label"`

	DayBucket string `json:"dayBucket,omitempty"`
	Role      string `json:"role,omitempty"`
	TaskKind  string `json:"taskKind,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	Status    string `json:"status,omitempty"`

	ArtifactID  string    `json:"artifactId,omitempty"`
	DisplayName string    `json:"displayName,omitempty"`
	VPath       string    `json:"vpath,omitempty"`
	DiskPath    string    `json:"diskPath,omitempty"`
	IsSummary   bool      `json:"isSummary,omitempty"`
	ProducedAt  time.Time `json:"producedAt,omitempty"`
}

// ArtifactListResult is the result for artifact.list.
type ArtifactListResult struct {
	Nodes []ArtifactNode `json:"nodes"`
}

// ArtifactSearchResult is the result for artifact.search.
type ArtifactSearchResult struct {
	Nodes      []ArtifactNode `json:"nodes"`
	MatchCount int            `json:"matchCount"`
}

// ArtifactGetResult is the result for artifact.get.
type ArtifactGetResult struct {
	Artifact  ArtifactNode `json:"artifact"`
	Content   string       `json:"content"`
	Truncated bool         `json:"truncated"`
	BytesRead int          `json:"bytesRead"`
}
