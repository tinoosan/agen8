package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
)

type sessionQueryService struct {
	server *RPCServer
}

func newSessionQueryService(server *RPCServer) *sessionQueryService {
	return &sessionQueryService{server: server}
}

func (q *sessionQueryService) sessionList(ctx context.Context, p protocol.SessionListParams) (protocol.SessionListResult, error) {
	srv := q.server
	if srv == nil {
		return protocol.SessionListResult{}, fmt.Errorf("rpc server is nil")
	}
	if _, err := srv.resolveThreadID(p.ThreadID); err != nil {
		return protocol.SessionListResult{}, err
	}
	filter := pkgstore.SessionFilter{
		TitleContains: strings.TrimSpace(p.TitleContains),
		ProjectRoot:   strings.TrimSpace(p.ProjectRoot),
		Limit:         clampLimit(p.Limit, 50, 500),
		Offset:        max(0, p.Offset),
		SortBy:        "updated_at",
		SortDesc:      true,
	}
	total, err := srv.session.CountSessions(ctx, filter)
	if err != nil {
		return protocol.SessionListResult{}, err
	}
	sessions, err := srv.session.ListSessionsPaginated(ctx, filter)
	if err != nil {
		return protocol.SessionListResult{}, err
	}
	runsBySessionID := map[string][]types.Run{}
	runsByID := map[string]types.Run{}
	if batchReader, ok := srv.session.(sessionRunBatchReader); ok {
		sessionIDs := make([]string, 0, len(sessions))
		for _, sess := range sessions {
			sessionID := strings.TrimSpace(sess.SessionID)
			if sessionID == "" {
				continue
			}
			sessionIDs = append(sessionIDs, sessionID)
		}
		if grouped, gerr := batchReader.ListRunsBySessionIDs(ctx, sessionIDs); gerr == nil {
			runsBySessionID = grouped
			for _, runs := range grouped {
				for _, run := range runs {
					runID := strings.TrimSpace(run.RunID)
					if runID == "" {
						continue
					}
					runsByID[runID] = run
				}
			}
		}
	}
	resolveRun := func(runID string) (types.Run, bool) {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			return types.Run{}, false
		}
		if run, ok := runsByID[runID]; ok {
			return run, true
		}
		run, rerr := srv.session.LoadRun(ctx, runID)
		if rerr != nil {
			return types.Run{}, false
		}
		runsByID[runID] = run
		return run, true
	}
	out := make([]protocol.SessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		mode := strings.TrimSpace(sess.Mode)
		teamID := strings.TrimSpace(sess.TeamID)
		profileID := strings.TrimSpace(sess.Profile)
		runID := strings.TrimSpace(sess.CurrentRunID)
		if runID == "" && len(sess.Runs) > 0 {
			runID = strings.TrimSpace(sess.Runs[0])
		}
		if runID != "" {
			if run, ok := resolveRun(runID); ok && run.Runtime != nil {
				if profileID == "" {
					profileID = strings.TrimSpace(run.Runtime.Profile)
				}
				if teamID == "" {
					teamID = strings.TrimSpace(run.Runtime.TeamID)
				}
			}
			if srv.agentService != nil {
				if _, inferredTeamID := srv.agentService.InferRunRoleAndTeam(ctx, runID); teamID == "" && inferredTeamID != "" {
					teamID = strings.TrimSpace(inferredTeamID)
				}
			}
		}
		if mode == "" {
			if teamID != "" {
				mode = "multi-agent"
			} else {
				mode = "single-agent"
			}
		}
		totalAgents := 0
		runningAgents := 0
		pausedAgents := 0
		sessionRunIDs := collectSessionRunIDs(sess)
		if sessionID := strings.TrimSpace(sess.SessionID); sessionID != "" {
			if groupedRuns, ok := runsBySessionID[sessionID]; ok && len(groupedRuns) > 0 {
				runIDSet := make(map[string]struct{}, len(sessionRunIDs)+len(groupedRuns))
				for _, listedRunID := range sessionRunIDs {
					listedRunID = strings.TrimSpace(listedRunID)
					if listedRunID == "" {
						continue
					}
					runIDSet[listedRunID] = struct{}{}
				}
				for _, groupedRun := range groupedRuns {
					groupedRunID := strings.TrimSpace(groupedRun.RunID)
					if groupedRunID == "" {
						continue
					}
					if _, ok := runIDSet[groupedRunID]; ok {
						continue
					}
					sessionRunIDs = append(sessionRunIDs, groupedRunID)
					runIDSet[groupedRunID] = struct{}{}
				}
			}
		}
		for _, listedRunID := range sessionRunIDs {
			listedRunID = strings.TrimSpace(listedRunID)
			if listedRunID == "" {
				continue
			}
			totalAgents++
			run, ok := resolveRun(listedRunID)
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(run.Status)) {
			case strings.ToLower(types.RunStatusRunning):
				runningAgents++
			case strings.ToLower(types.RunStatusPaused):
				pausedAgents++
			}
		}
		item := protocol.SessionListItem{
			SessionID:     strings.TrimSpace(sess.SessionID),
			Title:         strings.TrimSpace(sess.Title),
			CurrentRunID:  strings.TrimSpace(sess.CurrentRunID),
			ActiveModel:   strings.TrimSpace(sess.ActiveModel),
			Mode:          mode,
			TeamID:        teamID,
			Profile:       profileID,
			ProjectRoot:   strings.TrimSpace(sess.ProjectRoot),
			RunningAgents: runningAgents,
			PausedAgents:  pausedAgents,
			TotalAgents:   totalAgents,
		}
		if sess.CreatedAt != nil && !sess.CreatedAt.IsZero() {
			item.CreatedAt = sess.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if sess.UpdatedAt != nil && !sess.UpdatedAt.IsZero() {
			item.UpdatedAt = sess.UpdatedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	return protocol.SessionListResult{Sessions: out, TotalCount: total}, nil
}

func (q *sessionQueryService) sessionGetTotals(ctx context.Context, p protocol.SessionGetTotalsParams) (protocol.SessionGetTotalsResult, error) {
	srv := q.server
	if srv == nil {
		return protocol.SessionGetTotalsResult{}, fmt.Errorf("rpc server is nil")
	}
	scope, err := srv.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	if srv.taskService == nil {
		return protocol.SessionGetTotalsResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "task store not configured"}
	}

	out := protocol.SessionGetTotalsResult{
		PricingKnown: true,
	}
	if strings.TrimSpace(scope.teamID) == "" {
		if srv.session != nil && strings.TrimSpace(scope.sessionID) != "" {
			if sess, err := srv.session.LoadSession(ctx, strings.TrimSpace(scope.sessionID)); err == nil {
				out.TotalTokensIn = sess.InputTokens
				out.TotalTokensOut = sess.OutputTokens
				out.TotalTokens = out.TotalTokensIn + out.TotalTokensOut
				if out.TotalTokens == 0 {
					out.TotalTokens = sess.TotalTokens
				}
				out.TotalCostUSD = sess.CostUSD
				out.PricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRun(ctx, srv.session, strings.TrimSpace(scope.runID))
			}
		}
		stats, err := srv.taskService.GetRunStats(ctx, strings.TrimSpace(scope.runID))
		if err == nil {
			out.TasksDone = stats.Succeeded + stats.Failed
		}
		return out, nil
	}

	runIDSet := map[string]struct{}{}
	manifestRunIDs, _ := srv.loadTeamManifestRunRoles(ctx, strings.TrimSpace(scope.teamID))
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runIDSet[runID] = struct{}{}
	}
	if len(runIDSet) == 0 {
		tasks, err := srv.taskService.ListTasks(ctx, state.TaskFilter{
			TeamID:   strings.TrimSpace(scope.teamID),
			Limit:    1000,
			SortBy:   "created_at",
			SortDesc: true,
		})
		if err != nil {
			return protocol.SessionGetTotalsResult{}, err
		}
		for _, t := range tasks {
			if r := strings.TrimSpace(t.RunID); r != "" {
				runIDSet[r] = struct{}{}
			}
		}
	}
	tasks, err := srv.taskService.ListTasks(ctx, state.TaskFilter{
		TeamID:   strings.TrimSpace(scope.teamID),
		Limit:    2000,
		SortBy:   "created_at",
		SortDesc: true,
	})
	if err != nil {
		return protocol.SessionGetTotalsResult{}, err
	}
	for _, t := range tasks {
		if t.Status == types.TaskStatusSucceeded || t.Status == types.TaskStatusFailed || t.Status == types.TaskStatusCanceled {
			if len(runIDSet) == 0 {
				out.TasksDone++
				continue
			}
			if _, ok := runIDSet[strings.TrimSpace(t.RunID)]; ok {
				out.TasksDone++
			}
		}
	}
	statsTotalTokens := 0
	seenSessionIDs := map[string]struct{}{}
	for runID := range runIDSet {
		if srv.session != nil {
			if run, err := srv.session.LoadRun(ctx, runID); err == nil {
				if sessionID := strings.TrimSpace(run.SessionID); sessionID != "" {
					if _, seen := seenSessionIDs[sessionID]; !seen {
						seenSessionIDs[sessionID] = struct{}{}
						if sess, serr := srv.session.LoadSession(ctx, sessionID); serr == nil {
							out.TotalTokensIn += sess.InputTokens
							out.TotalTokensOut += sess.OutputTokens
						}
					}
				}
			}
		}
		rs, err := srv.taskService.GetRunStats(ctx, runID)
		if err != nil {
			continue
		}
		statsTotalTokens += rs.TotalTokens
		out.TotalCostUSD += rs.TotalCost
		if rs.TotalTokens > 0 && rs.TotalCost <= 0 && !pricingKnownForRun(ctx, srv.session, runID) {
			out.PricingKnown = false
		}
	}
	out.TotalTokens = out.TotalTokensIn + out.TotalTokensOut
	if out.TotalTokens == 0 {
		out.TotalTokens = statsTotalTokens
	}
	if out.TotalTokens == 0 {
		out.PricingKnown = true
	}
	return out, nil
}

func (q *sessionQueryService) activityList(ctx context.Context, p protocol.ActivityListParams) (protocol.ActivityListResult, error) {
	srv := q.server
	if srv == nil {
		return protocol.ActivityListResult{}, fmt.Errorf("rpc server is nil")
	}
	scope, err := srv.resolveTeamOrRunScope(ctx, p.ThreadID, p.TeamID, p.RunID)
	if err != nil {
		return protocol.ActivityListResult{}, err
	}
	limit := clampLimit(p.Limit, 200, 2000)
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	nextOffset := func(start, count, total int) int {
		if start+count < total {
			return start + count
		}
		return 0
	}
	batchReader, hasBatchReader := srv.session.(sessionActivityBatchReader)

	if strings.TrimSpace(scope.teamID) == "" {
		runID := strings.TrimSpace(scope.runID)
		if !p.IncludeChildRuns || runID == "" {
			acts, err := srv.session.ListActivities(ctx, runID, limit, offset)
			if err != nil {
				return protocol.ActivityListResult{}, err
			}
			parentRole := "agent"
			if srv.run.Runtime != nil && strings.TrimSpace(srv.run.Runtime.Profile) != "" {
				parentRole = strings.TrimSpace(srv.run.Runtime.Profile)
			}
			for i := range acts {
				if acts[i].Data == nil {
					acts[i].Data = map[string]string{}
				}
				if strings.TrimSpace(acts[i].Data["role"]) == "" {
					acts[i].Data["role"] = parentRole
				}
			}
			total, _ := srv.session.CountActivities(ctx, runID)
			return protocol.ActivityListResult{
				Activities: acts,
				TotalCount: total,
				NextOffset: nextOffset(offset, len(acts), total),
			}, nil
		}

		parentRole := "agent"
		if srv.run.Runtime != nil && strings.TrimSpace(srv.run.Runtime.Profile) != "" {
			parentRole = strings.TrimSpace(srv.run.Runtime.Profile)
		}
		roleByRunID := map[string]string{runID: parentRole}
		runIDs := []string{runID}
		children, childErr := srv.session.ListChildRuns(ctx, runID)
		if childErr == nil {
			for _, child := range children {
				childRunID := strings.TrimSpace(child.RunID)
				if childRunID == "" {
					continue
				}
				n := child.SpawnIndex
				if n <= 0 {
					n = 1
				}
				roleByRunID[childRunID] = fmt.Sprintf("Sub-agent %d", n)
				runIDs = append(runIDs, childRunID)
			}
		}
		if hasBatchReader {
			total, totalErr := batchReader.CountActivitiesByRunIDs(ctx, runIDs)
			if totalErr == nil {
				acts, listErr := batchReader.ListActivitiesByRunIDs(ctx, runIDs, limit, offset, p.SortDesc)
				if listErr == nil {
					for i := range acts {
						if acts[i].Data == nil {
							acts[i].Data = map[string]string{}
						}
						runForAct := strings.TrimSpace(acts[i].Data["runId"])
						role := strings.TrimSpace(roleByRunID[runForAct])
						if role == "" {
							role = parentRole
						}
						// Always override — the stored event role reflects the session's
						// profile name (e.g. "General Agent"), not its logical role in the
						// parent (e.g. "Sub-agent 1"). Mirror the fallback-path behaviour.
						acts[i].Data["role"] = role
					}
					return protocol.ActivityListResult{
						Activities: acts,
						TotalCount: total,
						NextOffset: nextOffset(offset, len(acts), total),
					}, nil
				}
			}
		}

		merged := make([]types.Activity, 0, 256)
		parentActs, err := srv.session.ListActivities(ctx, runID, 500, 0)
		if err == nil {
			for i := range parentActs {
				if parentActs[i].Data == nil {
					parentActs[i].Data = map[string]string{}
				}
				if strings.TrimSpace(parentActs[i].Data["role"]) == "" {
					parentActs[i].Data["role"] = parentRole
				}
			}
			merged = append(merged, parentActs...)
		}
		children, err = srv.session.ListChildRuns(ctx, runID)
		if err == nil {
			for _, child := range children {
				childRunID := strings.TrimSpace(child.RunID)
				if childRunID == "" {
					continue
				}
				n := child.SpawnIndex
				if n <= 0 {
					n = 1
				}
				childRole := fmt.Sprintf("Sub-agent %d", n)
				childActs, err := srv.session.ListActivities(ctx, childRunID, 300, 0)
				if err != nil {
					continue
				}
				for i := range childActs {
					if childActs[i].Data == nil {
						childActs[i].Data = map[string]string{}
					}
					childActs[i].Data["role"] = childRole
					merged = append(merged, childActs[i])
				}
			}
		}
		sort.SliceStable(merged, func(i, j int) bool {
			if p.SortDesc {
				return merged[i].StartedAt.After(merged[j].StartedAt)
			}
			return merged[i].StartedAt.Before(merged[j].StartedAt)
		})
		total := len(merged)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		out := []types.Activity{}
		if offset < end {
			out = append(out, merged[offset:end]...)
		}
		return protocol.ActivityListResult{
			Activities: out,
			TotalCount: total,
			NextOffset: nextOffset(offset, len(out), total),
		}, nil
	}

	manifestRunIDs, runRole := srv.loadTeamManifestRunRoles(ctx, strings.TrimSpace(scope.teamID))
	runSet := map[string]struct{}{}
	for _, runID := range manifestRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		runSet[runID] = struct{}{}
	}
	if len(runSet) == 0 {
		tasks, err := srv.taskService.ListTasks(ctx, state.TaskFilter{
			TeamID:   strings.TrimSpace(scope.teamID),
			Limit:    1000,
			SortBy:   "created_at",
			SortDesc: true,
		})
		if err != nil {
			return protocol.ActivityListResult{}, err
		}
		for _, t := range tasks {
			runID := strings.TrimSpace(t.RunID)
			if runID == "" {
				continue
			}
			runSet[runID] = struct{}{}
			if _, ok := runRole[runID]; !ok {
				runRole[runID] = strings.TrimSpace(t.AssignedRole)
			}
		}
	}
	if targetRunID := strings.TrimSpace(p.RunID); targetRunID != "" {
		if _, ok := runSet[targetRunID]; ok {
			runSet = map[string]struct{}{targetRunID: {}}
		} else {
			runSet = map[string]struct{}{}
		}
	}

	roleFilter := strings.TrimSpace(p.Role)
	runIDs := make([]string, 0, len(runSet))
	for runID := range runSet {
		role := strings.TrimSpace(runRole[runID])
		if roleFilter != "" && !strings.EqualFold(roleFilter, role) {
			continue
		}
		runIDs = append(runIDs, runID)
	}
	if hasBatchReader {
		total, totalErr := batchReader.CountActivitiesByRunIDs(ctx, runIDs)
		if totalErr == nil {
			acts, listErr := batchReader.ListActivitiesByRunIDs(ctx, runIDs, limit, offset, p.SortDesc)
			if listErr == nil {
				for i := range acts {
					if acts[i].Data == nil {
						acts[i].Data = map[string]string{}
					}
					runForAct := strings.TrimSpace(acts[i].Data["runId"])
					role := strings.TrimSpace(runRole[runForAct])
					if role != "" {
						acts[i].Data["role"] = role
					}
					if runForAct != "" {
						acts[i].ID = runForAct + ":" + acts[i].ID
					}
				}
				return protocol.ActivityListResult{
					Activities: acts,
					TotalCount: total,
					NextOffset: nextOffset(offset, len(acts), total),
				}, nil
			}
		}
	}

	merged := make([]types.Activity, 0, 512)
	for _, runID := range runIDs {
		acts, err := srv.session.ListActivities(ctx, runID, 300, 0)
		if err != nil {
			continue
		}
		role := strings.TrimSpace(runRole[runID])
		for i := range acts {
			if role != "" {
				if acts[i].Data == nil {
					acts[i].Data = map[string]string{}
				}
				acts[i].Data["role"] = role
			}
			acts[i].ID = runID + ":" + acts[i].ID
			merged = append(merged, acts[i])
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if p.SortDesc {
			return merged[i].StartedAt.After(merged[j].StartedAt)
		}
		return merged[i].StartedAt.Before(merged[j].StartedAt)
	})
	total := len(merged)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := []types.Activity{}
	if offset < end {
		out = append(out, merged[offset:end]...)
	}
	return protocol.ActivityListResult{
		Activities: out,
		TotalCount: total,
		NextOffset: nextOffset(offset, len(out), total),
	}, nil
}
