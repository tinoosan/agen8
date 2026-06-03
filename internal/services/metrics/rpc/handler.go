package rpc

import (
	"context"
	"errors"
	"strings"

	metricsApp "github.com/tinoosan/agen8-mcp-server/internal/services/metrics/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/metrics/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

// Handler adapts the metrics application service to RPC protocol types.
type Handler struct {
	svc *metricsApp.Service
}

// NewHandler creates an RPC handler wrapping the metrics application service.
func NewHandler(svc *metricsApp.Service) *Handler {
	return &Handler{svc: svc}
}

// ProjectSummary handles metrics.projectSummary RPC calls.
func (h *Handler) ProjectSummary(ctx context.Context, p protocol.MetricsProjectSummaryParams) (protocol.MetricsProjectSummaryResult, error) {
	projectRoot := strings.TrimSpace(p.ProjectRoot)
	if projectRoot == "" {
		return protocol.MetricsProjectSummaryResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "projectRoot is required"}
	}

	result, err := h.svc.ProjectSummary(ctx, projectRoot)
	if err != nil {
		return protocol.MetricsProjectSummaryResult{}, toProtocolError(err)
	}

	spaces := make([]protocol.MetricsSpaceSummary, len(result.Spaces))
	for i, t := range result.Spaces {
		spaces[i] = toProtocolSpaceSummary(t)
	}

	return protocol.MetricsProjectSummaryResult{
		TotalCostUSD:   result.Tokens.CostUSD,
		TotalTokensIn:  result.Tokens.InputTokens,
		TotalTokensOut: result.Tokens.OutputTokens,
		TotalTokens:    result.Tokens.TotalTokens(),
		TotalTasks:     result.TotalTasks,
		TotalRuns:      result.TotalRuns,
		ActiveSpaces:   result.ActiveSpaces,
		Spaces:         spaces,
	}, nil
}

// SpaceDetail handles metrics.spaceDetail RPC calls.
func (h *Handler) SpaceDetail(ctx context.Context, p protocol.MetricsSpaceDetailParams) (protocol.MetricsSpaceDetailResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return protocol.MetricsSpaceDetailResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "spaceId is required"}
	}

	result, err := h.svc.SpaceDetail(ctx, spaceID)
	if err != nil {
		return protocol.MetricsSpaceDetailResult{}, toProtocolError(err)
	}

	members := make([]protocol.MetricsMemberSummary, len(result.Members))
	for i, r := range result.Members {
		members[i] = toProtocolMemberSummary(r)
	}

	return protocol.MetricsSpaceDetailResult{
		SpaceID:        result.SpaceID,
		CostUSD:        result.Tokens.CostUSD,
		TotalTokens:    result.Tokens.TotalTokens(),
		TokensIn:       result.Tokens.InputTokens,
		TokensOut:      result.Tokens.OutputTokens,
		TasksDone:      result.TasksDone,
		TasksFailed:    result.TasksFailed,
		TasksPending:   result.TasksPending,
		AvgCostPerTask: result.AvgCostPerTask,
		AvgDurationMs:  result.AvgDurationMs,
		Members:        members,
	}, nil
}

// MemberDetail handles metrics.memberDetail RPC calls.
func (h *Handler) MemberDetail(ctx context.Context, p protocol.MetricsMemberDetailParams) (protocol.MetricsMemberDetailResult, error) {
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.MetricsMemberDetailResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}

	result, err := h.svc.MemberDetail(ctx, runID)
	if err != nil {
		return protocol.MetricsMemberDetailResult{}, toProtocolError(err)
	}

	toolUsage := make([]protocol.MetricsToolUsage, len(result.ToolUsage))
	for i, u := range result.ToolUsage {
		toolUsage[i] = toProtocolToolUsage(u)
	}

	recentTasks := make([]protocol.MetricsRecentTask, len(result.RecentTasks))
	for i, t := range result.RecentTasks {
		recentTasks[i] = toProtocolRecentTask(t)
	}

	return protocol.MetricsMemberDetailResult{
		Member:            result.Member,
		RunID:             result.RunID,
		CostUSD:           result.Tokens.CostUSD,
		TokensIn:          result.Tokens.InputTokens,
		TokensOut:         result.Tokens.OutputTokens,
		TotalTokens:       result.Tokens.TotalTokens(),
		TasksDone:         result.TasksDone,
		TasksFailed:       result.TasksFailed,
		AvgTaskDurationMs: result.AvgTaskDurationMs,
		SuccessRate:       result.SuccessRate,
		ToolUsage:         toolUsage,
		RecentTasks:       recentTasks,
	}, nil
}

// TimeSeries handles metrics.timeSeries RPC calls.
func (h *Handler) TimeSeries(ctx context.Context, p protocol.MetricsTimeSeriesParams) (protocol.MetricsTimeSeriesResult, error) {
	scope := strings.TrimSpace(p.Scope)
	scopeID := strings.TrimSpace(p.ScopeID)
	metric := strings.TrimSpace(p.Metric)

	if scope == "" || scopeID == "" || metric == "" {
		return protocol.MetricsTimeSeriesResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "scope, scopeId, and metric are required",
		}
	}

	result, err := h.svc.TimeSeries(ctx, metricsApp.TimeSeriesParams{
		Scope:       domain.Scope(scope),
		ScopeID:     scopeID,
		Metric:      domain.Metric(metric),
		Granularity: domain.Granularity(strings.TrimSpace(p.Granularity)),
		FromTime:    strings.TrimSpace(p.FromTime),
		ToTime:      strings.TrimSpace(p.ToTime),
	})
	if err != nil {
		return protocol.MetricsTimeSeriesResult{}, toProtocolError(err)
	}

	points := make([]protocol.MetricsTimeSeriesPoint, len(result.Points))
	for i, pt := range result.Points {
		points[i] = protocol.MetricsTimeSeriesPoint{T: pt.T, V: pt.V}
	}

	return protocol.MetricsTimeSeriesResult{Points: points, Total: result.Total}, nil
}

// ── error mapping ───────────────────────────────────

func toProtocolError(err error) error {
	var invalidInput *domain.ErrInvalidInput
	if errors.As(err, &invalidInput) {
		return &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: invalidInput.Error()}
	}
	var unavailable *domain.ErrServiceUnavailable
	if errors.As(err, &unavailable) {
		return &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: unavailable.Error()}
	}
	var protoErr *protocol.ProtocolError
	if errors.As(err, &protoErr) {
		return err
	}
	return &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: err.Error()}
}

// ── type mapping ────────────────────────────────────

func toProtocolSpaceSummary(s domain.SpaceSummary) protocol.MetricsSpaceSummary {
	return protocol.MetricsSpaceSummary{
		SpaceID:      s.SpaceID,
		CostUSD:      s.Tokens.CostUSD,
		TokensIn:     s.Tokens.InputTokens,
		TokensOut:    s.Tokens.OutputTokens,
		TotalTokens:  s.Tokens.TotalTokens(),
		TasksDone:    s.TasksDone,
		TasksFailed:  s.TasksFailed,
		TasksPending: s.TasksPending,
		MemberCount:  s.MemberCount,
		Status:       s.Status,
	}
}

func toProtocolMemberSummary(s domain.MemberSummary) protocol.MetricsMemberSummary {
	return protocol.MetricsMemberSummary{
		Member:         s.Member,
		RunID:          s.RunID,
		CostUSD:        s.Tokens.CostUSD,
		TokensIn:       s.Tokens.InputTokens,
		TokensOut:      s.Tokens.OutputTokens,
		TotalTokens:    s.Tokens.TotalTokens(),
		TasksDone:      s.TasksDone,
		TasksFailed:    s.TasksFailed,
		AvgCostPerTask: s.AvgCostPerTask,
	}
}

func toProtocolToolUsage(u domain.ToolUsage) protocol.MetricsToolUsage {
	return protocol.MetricsToolUsage{
		Tool:          u.Tool,
		Count:         u.Count,
		AvgDurationMs: u.AvgDurationMs,
	}
}

func toProtocolRecentTask(t domain.RecentTask) protocol.MetricsRecentTask {
	return protocol.MetricsRecentTask{
		TaskID:      t.TaskID,
		Goal:        t.Goal,
		Status:      t.Status,
		CostUSD:     t.CostUSD,
		DurationMs:  t.DurationMs,
		CompletedAt: t.CompletedAt,
	}
}
