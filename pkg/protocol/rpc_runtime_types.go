package protocol

import "github.com/tinoosan/agen8-mcp-server/pkg/types"

type LogsQueryParams struct {
	Cwd         string   `json:"cwd,omitempty"`
	ProjectRoot string   `json:"projectRoot,omitempty"`
	RunID       RunID    `json:"runId,omitempty"`
	SpaceID     SpaceID  `json:"spaceId,omitempty"`
	MemberID    string   `json:"memberId,omitempty"`
	Types       []string `json:"types,omitempty"`
	Severities  []string `json:"severities,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Search      string   `json:"search,omitempty"`
	Origin      string   `json:"origin,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Offset      int      `json:"offset,omitempty"`
	AfterSeq    int64    `json:"afterSeq,omitempty"`
	SortDesc    bool     `json:"sortDesc,omitempty"`
}

type LogsQueryResult struct {
	Events []LogEntry `json:"events"`
	Next   int64      `json:"next,omitempty"`
}

type LogEntry struct {
	EventID   string            `json:"eventId"`
	RunID     string            `json:"runId,omitempty"`
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Origin    string            `json:"origin,omitempty"`
	Severity  string            `json:"severity"`
	Category  string            `json:"category"`
	Actor     string            `json:"actor,omitempty"`
	Scope     string            `json:"scope,omitempty"`
	Summary   string            `json:"summary"`
	Details   []string          `json:"details,omitempty"`
	TypeLabel string            `json:"typeLabel,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
}

type EventStreamParams struct {
	RunID    RunID    `json:"runId,omitempty"`
	AfterSeq int64    `json:"afterSeq,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Types    []string `json:"types,omitempty"`
}

type EventStreamResult struct {
	Events    []types.EventRecord `json:"events"`
	Next      int64               `json:"next,omitempty"`
	LatestSeq int64               `json:"latestSeq,omitempty"`
}

type RuntimeGetRunStateParams struct {
	SpaceID  SpaceID `json:"spaceId,omitempty"`
	MemberID string  `json:"memberId,omitempty"`
	RunID    RunID   `json:"runId"`
}

type RuntimeGetSpaceStateParams struct {
	SpaceID SpaceID `json:"spaceId"`
}

type RuntimeRunState struct {
	SpaceID         SpaceID `json:"spaceId,omitempty"`
	MemberID        string  `json:"memberId,omitempty"`
	RunID           RunID   `json:"runId"`
	Model           string  `json:"model,omitempty"`
	RuntimeKind     string  `json:"runtimeKind,omitempty"`
	RunTotalTokens  int     `json:"runTotalTokens,omitempty"`
	RunTotalCostUSD float64 `json:"runTotalCostUSD,omitempty"`
	PersistedStatus string  `json:"persistedStatus,omitempty"`
	WorkerPresent   bool    `json:"workerPresent"`
	ClosedFlag      bool    `json:"closedFlag"`
	LastHeartbeatAt string  `json:"lastHeartbeatAt,omitempty"`
	EffectiveStatus string  `json:"effectiveStatus,omitempty"`
}

type RuntimeGetRunStateResult struct {
	State RuntimeRunState `json:"state"`
}

type RuntimeGetSpaceStateResult struct {
	SpaceID SpaceID           `json:"spaceId"`
	Runs    []RuntimeRunState `json:"runs"`
}
