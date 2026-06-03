package protocol

import "github.com/tinoosan/agen8-mcp-server/pkg/types"

type EventsListPaginatedParams struct {
	Cwd         string   `json:"cwd,omitempty"`
	ProjectRoot string   `json:"projectRoot,omitempty"`
	RunID       string   `json:"runId"`
	SpaceID     string   `json:"spaceId,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Offset      int      `json:"offset,omitempty"`
	AfterSeq    int64    `json:"afterSeq,omitempty"`
	BeforeSeq   int64    `json:"beforeSeq,omitempty"`
	Types       []string `json:"types,omitempty"`
	SortDesc    bool     `json:"sortDesc,omitempty"`
	Severities  []string `json:"severities,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Search      string   `json:"search,omitempty"`
	Origin      string   `json:"origin,omitempty"`
}

type EventsListPaginatedResult struct {
	Events []types.EventRecord `json:"events"`
	Next   int64               `json:"next,omitempty"`
}

type EventsLatestSeqParams struct {
	RunID string `json:"runId"`
}

type EventsLatestSeqResult struct {
	Seq int64 `json:"seq"`
}

type EventsCountParams struct {
	Cwd         string   `json:"cwd,omitempty"`
	ProjectRoot string   `json:"projectRoot,omitempty"`
	RunID       string   `json:"runId"`
	SpaceID     string   `json:"spaceId,omitempty"`
	Types       []string `json:"types,omitempty"`
}

type EventsCountResult struct {
	Count int `json:"count"`
}


