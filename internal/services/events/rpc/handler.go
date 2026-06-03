package rpc

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// ProjectSpaceInfo contains the minimal space information needed by the events handler.
type ProjectSpaceInfo struct {
	SpaceID          string
	ProjectID        string
	CoordinatorRunID string
	Members          []ProjectMemberInfo
}

// ProjectMemberInfo contains the run binding for a role in a project space.
type ProjectMemberInfo struct {
	MemberLabel string
	RunID       string
}

// ProjectSpaceLister lists spaces for a project root.
type ProjectSpaceLister interface {
	ListSpaces(ctx context.Context, projectRoot string) ([]ProjectSpaceInfo, error)
}

// ProjectRootResolverFunc resolves a project root from cwd and explicit root.
type ProjectRootResolverFunc func(cwd, explicitRoot string) (string, error)

// Handler implements the RPC event methods.
type Handler struct {
	Events             domain.EventService
	SpaceLister        ProjectSpaceLister
	ResolveProjectRoot ProjectRootResolverFunc
}

var errEventsServiceNotConfigured = errors.New("events service not configured")

// ListPaginated handles events.listPaginated.
func (h *Handler) ListPaginated(ctx context.Context, p protocol.EventsListPaginatedParams) (protocol.EventsListPaginatedResult, error) {
	if h.Events == nil {
		return protocol.EventsListPaginatedResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		projectRoot := h.resolveProjectRoot(p.ProjectRoot, p.Cwd)
		if projectRoot == "" {
			return protocol.EventsListPaginatedResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId or projectRoot is required"}
		}
		return h.projectEventsListPaginated(ctx, projectRoot, strings.TrimSpace(p.SpaceID), p)
	}
	filter := domain.EventFilter{
		RunID:      runID,
		Limit:      p.Limit,
		Offset:     p.Offset,
		AfterSeq:   p.AfterSeq,
		BeforeSeq:  p.BeforeSeq,
		Types:      p.Types,
		SortDesc:   p.SortDesc,
		Severities: p.Severities,
		Categories: p.Categories,
		Search:     strings.TrimSpace(p.Search),
		Origin:     strings.TrimSpace(p.Origin),
	}
	events, next, err := h.Events.ListPaginated(ctx, filter)
	if err != nil {
		return protocol.EventsListPaginatedResult{}, err
	}
	return protocol.EventsListPaginatedResult{Events: events, Next: next}, nil
}

// LatestSeq handles events.latestSeq.
func (h *Handler) LatestSeq(ctx context.Context, p protocol.EventsLatestSeqParams) (protocol.EventsLatestSeqResult, error) {
	if h.Events == nil {
		return protocol.EventsLatestSeqResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		return protocol.EventsLatestSeqResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId is required"}
	}
	seq, err := h.Events.LatestSeq(ctx, runID)
	if err != nil {
		return protocol.EventsLatestSeqResult{}, err
	}
	return protocol.EventsLatestSeqResult{Seq: seq}, nil
}

// Count handles events.count.
func (h *Handler) Count(ctx context.Context, p protocol.EventsCountParams) (protocol.EventsCountResult, error) {
	if h.Events == nil {
		return protocol.EventsCountResult{}, errEventsServiceNotConfigured
	}
	runID := strings.TrimSpace(p.RunID)
	if runID == "" {
		projectRoot := h.resolveProjectRoot(p.ProjectRoot, p.Cwd)
		if projectRoot == "" {
			return protocol.EventsCountResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidParams, Message: "runId or projectRoot is required"}
		}
		events, _, err := h.projectEvents(ctx, projectRoot, strings.TrimSpace(p.SpaceID), domain.EventFilter{Types: p.Types})
		if err != nil {
			return protocol.EventsCountResult{}, err
		}
		return protocol.EventsCountResult{Count: len(events)}, nil
	}
	filter := domain.EventFilter{RunID: runID, Types: p.Types}
	count, err := h.Events.Count(ctx, filter)
	if err != nil {
		return protocol.EventsCountResult{}, err
	}
	return protocol.EventsCountResult{Count: count}, nil
}

func (h *Handler) resolveProjectRoot(projectRoot, cwd string) string {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot != "" {
		return projectRoot
	}
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || h.ResolveProjectRoot == nil {
		return ""
	}
	if resolved, err := h.ResolveProjectRoot(cwd, ""); err == nil {
		return resolved
	}
	return ""
}

func (h *Handler) projectEventsListPaginated(ctx context.Context, projectRoot, spaceID string, p protocol.EventsListPaginatedParams) (protocol.EventsListPaginatedResult, error) {
	baseFilter := domain.EventFilter{
		Limit:      clampLimit(p.Limit, 100, 2000),
		Offset:     max(0, p.Offset),
		Types:      p.Types,
		SortDesc:   p.SortDesc,
		Severities: p.Severities,
		Categories: p.Categories,
		Search:     strings.TrimSpace(p.Search),
		Origin:     strings.TrimSpace(p.Origin),
	}
	events, next, err := h.projectEvents(ctx, projectRoot, spaceID, baseFilter)
	if err != nil {
		return protocol.EventsListPaginatedResult{}, err
	}
	return protocol.EventsListPaginatedResult{Events: events, Next: next}, nil
}

func (h *Handler) projectEvents(ctx context.Context, projectRoot, spaceID string, baseFilter domain.EventFilter) ([]types.EventRecord, int64, error) {
	if h.SpaceLister == nil {
		return nil, 0, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "project space service is not configured"}
	}
	spaces, err := h.SpaceLister.ListSpaces(ctx, strings.TrimSpace(projectRoot))
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]ProjectSpaceInfo, 0, len(spaces))
	for _, space := range spaces {
		if strings.TrimSpace(spaceID) != "" && !projectSpaceMatchesFilter(space, spaceID) {
			continue
		}
		if len(projectSpaceRuns(space)) == 0 {
			continue
		}
		filtered = append(filtered, space)
	}
	if strings.TrimSpace(spaceID) != "" && len(filtered) == 0 {
		return nil, 0, &protocol.ProtocolError{Code: protocol.CodeItemNotFound, Message: "space not found in project"}
	}
	pageSize := baseFilter.Limit
	if pageSize <= 0 {
		pageSize = 100
	}
	sortDesc := baseFilter.SortDesc
	all := make([]types.EventRecord, 0, len(filtered)*min(pageSize, 500))
	for _, space := range filtered {
		for _, run := range projectSpaceRuns(space) {
			f := baseFilter
			f.RunID = run.RunID
			f.Limit = pageSize
			rows, _, err := h.Events.ListPaginated(ctx, f)
			if err != nil {
				return nil, 0, err
			}
			for _, event := range rows {
				all = append(all, annotateProjectEvent(event, space, run))
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Timestamp.Equal(all[j].Timestamp) {
			if sortDesc {
				return all[i].Timestamp.After(all[j].Timestamp)
			}
			return all[i].Timestamp.Before(all[j].Timestamp)
		}
		if sortDesc {
			return strings.TrimSpace(string(all[i].EventID)) > strings.TrimSpace(string(all[j].EventID))
		}
		return strings.TrimSpace(string(all[i].EventID)) < strings.TrimSpace(string(all[j].EventID))
	})
	offset := baseFilter.Offset
	limit := baseFilter.Limit
	if offset > len(all) {
		offset = len(all)
	}
	end := len(all)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	next := int64(0)
	if end < len(all) {
		next = int64(end)
	}
	return append([]types.EventRecord(nil), all[offset:end]...), next, nil
}

func projectSpaceMatchesFilter(space ProjectSpaceInfo, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(space.SpaceID), filter)
}

func projectSpaceRuns(space ProjectSpaceInfo) []ProjectMemberInfo {
	seen := map[string]struct{}{}
	out := make([]ProjectMemberInfo, 0, len(space.Members)+1)
	add := func(roleName, runID string) {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			return
		}
		key := strings.ToLower(runID)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, ProjectMemberInfo{
			MemberLabel: strings.TrimSpace(roleName),
			RunID:       runID,
		})
	}
	for _, role := range space.Members {
		add(role.MemberLabel, role.RunID)
	}
	add("coordinator", space.CoordinatorRunID)
	return out
}

func annotateProjectEvent(event types.EventRecord, space ProjectSpaceInfo, run ProjectMemberInfo) types.EventRecord {
	out := event
	data := make(map[string]string, len(event.Data)+5)
	for k, v := range event.Data {
		data[k] = v
	}
	if strings.TrimSpace(data["spaceId"]) == "" {
		data["spaceId"] = strings.TrimSpace(space.SpaceID)
	}
	if strings.TrimSpace(data["projectId"]) == "" {
		data["projectId"] = strings.TrimSpace(space.ProjectID)
	}
	if strings.TrimSpace(data["role"]) == "" {
		data["role"] = strings.TrimSpace(run.MemberLabel)
	}
	if strings.TrimSpace(data["runId"]) == "" {
		data["runId"] = strings.TrimSpace(run.RunID)
	}
	out.Data = data
	return out
}

func clampLimit(v, dflt, maxV int) int {
	if v <= 0 {
		return dflt
	}
	if v > maxV {
		return maxV
	}
	return v
}
