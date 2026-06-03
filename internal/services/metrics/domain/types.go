package domain

import "time"

// TokenCost is the core value object for token/cost aggregation.
type TokenCost struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

func (t TokenCost) TotalTokens() int { return t.InputTokens + t.OutputTokens }

func (t TokenCost) IsZero() bool {
	return t.InputTokens == 0 && t.OutputTokens == 0 && t.CostUSD == 0
}

func (t TokenCost) Add(o TokenCost) TokenCost {
	return TokenCost{
		InputTokens:  t.InputTokens + o.InputTokens,
		OutputTokens: t.OutputTokens + o.OutputTokens,
		CostUSD:      t.CostUSD + o.CostUSD,
	}
}

// SpaceSummary is the domain projection for project-level space metrics.
type SpaceSummary struct {
	SpaceID      string
	Status       string
	Tokens       TokenCost
	TasksDone    int
	TasksFailed  int
	TasksPending int
	MemberCount  int
}

// ProjectSummary is the domain projection for project-level metrics.
type ProjectSummary struct {
	Tokens       TokenCost
	TotalTasks   int
	TotalRuns    int
	ActiveSpaces int
	Spaces       []SpaceSummary
}

// MemberSummary is per-member metrics within a space.
type MemberSummary struct {
	Member         string
	RunID          string
	Tokens         TokenCost
	TasksDone      int
	TasksFailed    int
	AvgCostPerTask float64
}

// SpaceDetail is the domain projection for space-level detail.
type SpaceDetail struct {
	SpaceID        string
	Tokens         TokenCost
	TasksDone      int
	TasksFailed    int
	TasksPending   int
	AvgCostPerTask float64
	AvgDurationMs  int64
	Members        []MemberSummary
}

// ToolUsage describes tool invocation counts.
type ToolUsage struct {
	Tool          string
	Count         int
	AvgDurationMs int64
}

// RecentTask is a completed task snapshot.
type RecentTask struct {
	TaskID      string
	Goal        string
	Status      string
	CompletedAt string
	CostUSD     float64
	DurationMs  int64
}

// MemberDetail is the domain projection for member-level metrics.
type MemberDetail struct {
	Member            string
	RunID             string
	Tokens            TokenCost
	TasksDone         int
	TasksFailed       int
	AvgTaskDurationMs int64
	SuccessRate       float64
	ToolUsage         []ToolUsage
	RecentTasks       []RecentTask
}

// TimeSeriesPoint is a single bucket in a time series.
type TimeSeriesPoint struct {
	T string
	V float64
}

// TimeSeries is the result of a time-series query.
type TimeSeries struct {
	Points []TimeSeriesPoint
	Total  float64
}

// Scope identifies what entity metrics are scoped to.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeSpace   Scope = "space"
	ScopeMember  Scope = "member"
)

// Metric identifies which metric to query for time series.
type Metric string

const (
	MetricCost      Metric = "cost"
	MetricTokens    Metric = "tokens"
	MetricTokensIn  Metric = "tokensIn"
	MetricTokensOut Metric = "tokensOut"
	MetricCalls     Metric = "calls"
	MetricTasks     Metric = "tasks"
)

// Granularity for time bucketing.
type Granularity string

const (
	GranularityMinute Granularity = "minute"
	GranularityHour   Granularity = "hour"
	GranularityDay    Granularity = "day"
)

// TimeRange constrains time-series queries.
type TimeRange struct {
	From time.Time
	To   time.Time
}
