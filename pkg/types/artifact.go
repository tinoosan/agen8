package types

import "time"

// ArtifactNode is a tree-ready artifact row for browsing/searching deliverables.
type ArtifactNode struct {
	NodeKey   string `json:"nodeKey"`
	ParentKey string `json:"parentKey,omitempty"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`

	SpaceID string `json:"spaceId,omitempty"`
	RunID   string `json:"runId,omitempty"`

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
