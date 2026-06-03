package protocol

const (
	MethodOperatorMetricsSummary = "operatorMetrics.summary"
)

// OperatorMetricsSummaryParams requests operator metrics for a project.
type OperatorMetricsSummaryParams struct {
	ProjectID string `json:"projectId"`
}

// OperatorMetricsSummaryResult contains aggregated operator metrics.
type OperatorMetricsSummaryResult struct {
	ResolvedToday   int     `json:"resolvedToday"`   // Escalations resolved + OAs completed in last 24h
	OverdueCount    int     `json:"overdueCount"`    // Items past deadline
	AvgResolutionMs int64   `json:"avgResolutionMs"` // Average resolution time in milliseconds
	PrevAvgMs       int64   `json:"prevAvgMs"`       // Previous 7-day average for trend
	TrendDirection  string  `json:"trendDirection"`  // "faster", "slower", "stable"
	EscalationRatio float64 `json:"escalationRatio"` // 0.0-1.0, fraction that are escalations vs OAs
}
