package protocol

type MetricsProjectSummaryParams struct {
	ProjectRoot string `json:"projectRoot"`
}

// MetricsSpaceSummary is the per-space summary included in the project summary.
type MetricsSpaceSummary struct {
	SpaceID      string  `json:"spaceId"`
	CostUSD      float64 `json:"costUSD"`
	TokensIn     int     `json:"tokensIn"`
	TokensOut    int     `json:"tokensOut"`
	TotalTokens  int     `json:"totalTokens"`
	TasksDone    int     `json:"tasksDone"`
	TasksFailed  int     `json:"tasksFailed"`
	TasksPending int     `json:"tasksPending"`
	MemberCount  int     `json:"memberCount"`
	Status       string  `json:"status"`
}

// MetricsProjectSummaryResult is the result of metrics.projectSummary.
type MetricsProjectSummaryResult struct {
	TotalCostUSD   float64               `json:"totalCostUSD"`
	TotalTokensIn  int                   `json:"totalTokensIn"`
	TotalTokensOut int                   `json:"totalTokensOut"`
	TotalTokens    int                   `json:"totalTokens"`
	TotalTasks     int                   `json:"totalTasks"`
	TotalRuns      int                   `json:"totalRuns"`
	ActiveSpaces   int                   `json:"activeSpaces"`
	Spaces         []MetricsSpaceSummary `json:"spaces"`
}

// MetricsSpaceDetailParams are the params for metrics.spaceDetail.
type MetricsSpaceDetailParams struct {
	SpaceID string `json:"spaceId"`
}

// MetricsMemberSummary is the per-member summary within a space detail.
type MetricsMemberSummary struct {
	Member         string  `json:"member"`
	RunID          string  `json:"runId"`
	CostUSD        float64 `json:"costUSD"`
	TokensIn       int     `json:"tokensIn"`
	TokensOut      int     `json:"tokensOut"`
	TotalTokens    int     `json:"totalTokens"`
	TasksDone      int     `json:"tasksDone"`
	TasksFailed    int     `json:"tasksFailed"`
	AvgCostPerTask float64 `json:"avgCostPerTask"`
}

// MetricsSpaceDetailResult is the result of metrics.spaceDetail.
type MetricsSpaceDetailResult struct {
	SpaceID        string                 `json:"spaceId"`
	CostUSD        float64                `json:"costUSD"`
	TotalTokens    int                    `json:"totalTokens"`
	TokensIn       int                    `json:"tokensIn"`
	TokensOut      int                    `json:"tokensOut"`
	TasksDone      int                    `json:"tasksDone"`
	TasksFailed    int                    `json:"tasksFailed"`
	TasksPending   int                    `json:"tasksPending"`
	AvgCostPerTask float64                `json:"avgCostPerTask"`
	AvgDurationMs  int64                  `json:"avgDurationMs"`
	Members        []MetricsMemberSummary `json:"members"`
}

// MetricsMemberDetailParams are the params for metrics.memberDetail.
type MetricsMemberDetailParams struct {
	RunID            string `json:"runId"`
	IncludeChildRuns bool   `json:"includeChildRuns,omitempty"`
}

// MetricsToolUsage describes how many times a tool was used and the average duration.
type MetricsToolUsage struct {
	Tool          string `json:"tool"`
	Count         int    `json:"count"`
	AvgDurationMs int64  `json:"avgDurationMs"`
}

// MetricsRecentTask describes a recent task for the member detail view.
type MetricsRecentTask struct {
	TaskID      string  `json:"taskId"`
	Goal        string  `json:"goal"`
	Status      string  `json:"status"`
	CostUSD     float64 `json:"costUSD"`
	DurationMs  int64   `json:"durationMs"`
	CompletedAt string  `json:"completedAt,omitempty"`
}

// MetricsMemberDetailResult is the result of metrics.memberDetail.
type MetricsMemberDetailResult struct {
	Member            string              `json:"member"`
	RunID             string              `json:"runId"`
	CostUSD           float64             `json:"costUSD"`
	TokensIn          int                 `json:"tokensIn"`
	TokensOut         int                 `json:"tokensOut"`
	TotalTokens       int                 `json:"totalTokens"`
	TasksDone         int                 `json:"tasksDone"`
	TasksFailed       int                 `json:"tasksFailed"`
	AvgTaskDurationMs int64               `json:"avgTaskDurationMs"`
	SuccessRate       float64             `json:"successRate"`
	ToolUsage         []MetricsToolUsage  `json:"toolUsage"`
	RecentTasks       []MetricsRecentTask `json:"recentTasks"`
}

// MetricsTimeSeriesParams are the params for metrics.timeSeries.
type MetricsTimeSeriesParams struct {
	Scope       string `json:"scope"`       // "project" | "space" | "member"
	ScopeID     string `json:"scopeId"`     // projectRoot | spaceId | runId
	Metric      string `json:"metric"`      // "cost" | "tokens" | "tokensIn" | "tokensOut" | "tasks"
	Granularity string `json:"granularity"` // "hour" | "day"
	FromTime    string `json:"fromTime,omitempty"`
	ToTime      string `json:"toTime,omitempty"`
}

// MetricsTimeSeriesPoint is a single data point in a time series.
type MetricsTimeSeriesPoint struct {
	T string  `json:"t"` // ISO 8601 timestamp
	V float64 `json:"v"` // value
}

// MetricsTimeSeriesResult is the result of metrics.timeSeries.
type MetricsTimeSeriesResult struct {
	Points []MetricsTimeSeriesPoint `json:"points"`
	Total  float64                  `json:"total"`
}
