package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/internal/store"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func pricingKnownForRunID(cfg config.Config, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	run, err := store.LoadRun(cfg, runID)
	if err != nil {
		return false
	}
	if run.Runtime == nil {
		return false
	}
	if run.Runtime.PriceInPerMTokensUSD != 0 || run.Runtime.PriceOutPerMTokensUSD != 0 {
		return true
	}
	modelID := strings.TrimSpace(run.Runtime.Model)
	if modelID == "" {
		return false
	}
	_, _, ok := cost.DefaultPricing().Lookup(modelID)
	return ok
}

func loadTeamManifestFromDisk(cfg config.Config, teamID string) (*teamManifestFile, error) {
	path := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, strings.TrimSpace(teamID)), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest teamManifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (m *monitorModel) rpcRun() types.Run {
	if m.isDetached() {
		return types.Run{
			RunID:     "detached-control",
			SessionID: "detached-control",
		}
	}
	runID := strings.TrimSpace(m.runID)
	if strings.TrimSpace(m.teamID) != "" {
		if r := strings.TrimSpace(m.teamCoordinatorRunID); r != "" {
			runID = r
		}
	}
	sessionID := strings.TrimSpace(m.sessionID)
	if sessionID == "" && strings.TrimSpace(m.teamID) != "" {
		if manifest, err := loadTeamManifestFromDisk(m.cfg, m.teamID); err == nil && manifest != nil {
			sessionID = resolveTeamControlSessionID(manifest, sessionID)
			if sessionID != "" {
				m.sessionID = sessionID
			}
		}
	}
	if sessionID == "" {
		if strings.TrimSpace(m.teamID) != "" {
			sessionID = "team-" + strings.TrimSpace(m.teamID)
		} else {
			sessionID = "sess-" + strings.TrimSpace(m.runID)
		}
	}
	return types.Run{RunID: runID, SessionID: sessionID}
}

func (m *monitorModel) resolveTeamControlSessionID() string {
	if m == nil {
		return ""
	}
	sessionID := strings.TrimSpace(m.sessionID)
	if sessionID != "" {
		return sessionID
	}
	teamID := strings.TrimSpace(m.teamID)
	if teamID == "" {
		return ""
	}
	if manifest, err := loadTeamManifestFromDisk(m.cfg, teamID); err == nil && manifest != nil {
		sessionID = resolveTeamControlSessionID(manifest, "")
		if sessionID != "" {
			m.sessionID = sessionID
		}
	}
	return strings.TrimSpace(sessionID)
}

func (m *monitorModel) rpcRoundTrip(method string, params any, out any) error {
	if m == nil {
		return fmt.Errorf("monitor is nil")
	}
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	cli := protocol.TCPClient{
		Endpoint: strings.TrimSpace(m.rpcEndpoint),
		Timeout:  2 * time.Second,
	}
	if err := cli.Call(ctx, method, params, out); err != nil {
		return fmt.Errorf("rpc %s: %w", method, err)
	}
	return nil
}

func monitorRPCEndpoint() string {
	v := strings.TrimSpace(os.Getenv("WORKBENCH_RPC_ENDPOINT"))
	if v != "" {
		return v
	}
	return protocol.DefaultRPCEndpoint
}

func ensureRPCReachable(ctx context.Context) error {
	return pingRPCEndpoint(ctx, monitorRPCEndpoint())
}

func (m *monitorModel) checkRPCHealthCmd(manual bool) tea.Cmd {
	endpoint := strings.TrimSpace(m.rpcEndpoint)
	if endpoint == "" {
		endpoint = monitorRPCEndpoint()
	}
	return func() tea.Msg {
		baseCtx := m.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
		defer cancel()
		if err := pingRPCEndpoint(ctx, endpoint); err != nil {
			return rpcHealthMsg{reachable: false, err: err, manual: manual}
		}
		return rpcHealthMsg{reachable: true, manual: manual}
	}
}

func pingRPCEndpoint(ctx context.Context, endpoint string) error {
	p := protocol.SessionListParams{
		ThreadID: protocol.ThreadID("detached-control"),
		Limit:    1,
		Offset:   0,
	}
	var out protocol.SessionListResult
	cli := protocol.TCPClient{
		Endpoint: strings.TrimSpace(endpoint),
		Timeout:  2 * time.Second,
	}
	if err := cli.Call(ctx, protocol.MethodSessionList, p, &out); err != nil {
		return fmt.Errorf("daemon RPC unavailable at %s: %w", strings.TrimSpace(endpoint), err)
	}
	return nil
}

func (m *monitorModel) rpcControlSetModel(ctx context.Context, threadID, target, model string) ([]string, error) {
	if strings.TrimSpace(threadID) != strings.TrimSpace(m.rpcRun().SessionID) {
		return nil, fmt.Errorf("thread not found")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)

	if strings.TrimSpace(m.teamID) != "" {
		return m.queueTeamModelChange(model, target, "rpc.control.setModel")
	}

	runID := strings.TrimSpace(m.runID)
	if target != "" && target != runID && target != strings.TrimSpace(m.sessionID) {
		return nil, fmt.Errorf("target does not match active run")
	}
	ss, err := store.NewSQLiteSessionStore(m.cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.sessionID) != "" {
		if sess, serr := ss.LoadSession(ctx, strings.TrimSpace(m.sessionID)); serr == nil {
			sess.ActiveModel = model
			_ = ss.SaveSession(ctx, sess)
		}
	}
	return []string{runID}, nil
}

func (m *monitorModel) rpcControlSetProfile(_ context.Context, threadID, target, profileRef string) ([]string, error) {
	if strings.TrimSpace(threadID) != strings.TrimSpace(m.rpcRun().SessionID) {
		return nil, fmt.Errorf("thread not found")
	}
	profileRef = strings.TrimSpace(profileRef)
	if profileRef == "" {
		return nil, fmt.Errorf("profile is required")
	}
	target = strings.TrimSpace(target)
	if strings.TrimSpace(m.teamID) != "" {
		return nil, fmt.Errorf("profile switching is not supported in team mode")
	}
	runID := strings.TrimSpace(m.runID)
	if target != "" && target != runID && target != strings.TrimSpace(m.sessionID) {
		return nil, fmt.Errorf("target does not match active run")
	}
	return []string{runID}, nil
}

func (m *monitorModel) loadSessionTotalsCmd() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	return func() tea.Msg {
		params := protocol.SessionGetTotalsParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			if strings.TrimSpace(m.focusedRunID) != "" {
				params.RunID = strings.TrimSpace(m.focusedRunID)
			}
		} else {
			params.RunID = strings.TrimSpace(m.runID)
		}
		var res protocol.SessionGetTotalsResult
		if err := m.rpcRoundTrip(protocol.MethodSessionGetTotals, params, &res); err != nil {
			return sessionTotalsLoadedMsg{err: err}
		}
		now := time.Now().UTC()
		return sessionTotalsLoadedMsg{session: types.Session{
			SessionID:    strings.TrimSpace(m.rpcRun().SessionID),
			CreatedAt:    &now,
			InputTokens:  res.TotalTokensIn,
			OutputTokens: res.TotalTokensOut,
			TotalTokens:  res.TotalTokens,
			CostUSD:      res.TotalCostUSD,
		}, pricingKnown: res.PricingKnown, tasksDone: res.TasksDone}
	}
}

func (m *monitorModel) loadInboxPage() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	pageSize := m.inboxPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := m.inboxPage
	if page < 0 {
		page = 0
	}
	prevTasks := append([]taskState(nil), m.inboxList...)
	prevTotal := m.inboxTotalCount
	prevPage := m.inboxPage

	return func() tea.Msg {
		fetch := func(targetPage int) (protocol.TaskListResult, error) {
			params := protocol.TaskListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				View:     "inbox",
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.TaskListResult
			if err := m.rpcRoundTrip(protocol.MethodTaskList, params, &res); err != nil {
				return protocol.TaskListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return inboxLoadedMsg{tasks: []taskState{}, totalCount: 0, page: 0}
		}
		requestedPage := page
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}

		out := make([]taskState, 0, len(res.Tasks))
		for _, t := range res.Tasks {
			status := strings.TrimSpace(t.Status)
			if status != string(types.TaskStatusPending) && status != string(types.TaskStatusActive) {
				continue
			}
			ts := taskState{
				TaskID:       strings.TrimSpace(t.ID),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       status,
			}
			if !t.CreatedAt.IsZero() {
				ts.StartedAt = t.CreatedAt
			}
			if ts.TaskID != "" {
				out = append(out, ts)
			}
		}
		return inboxLoadedMsg{tasks: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) loadOutboxPage() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	pageSize := m.outboxPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := m.outboxPage
	if page < 0 {
		page = 0
	}
	prevEntries := append([]outboxEntry(nil), m.outboxResults...)
	prevTotal := m.outboxTotalCount
	prevPage := m.outboxPage

	return func() tea.Msg {
		fetch := func(targetPage int) (protocol.TaskListResult, error) {
			params := protocol.TaskListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				View:     "outbox",
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.TaskListResult
			if err := m.rpcRoundTrip(protocol.MethodTaskList, params, &res); err != nil {
				return protocol.TaskListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return outboxLoadedMsg{entries: []outboxEntry{}, totalCount: 0, page: 0}
		}
		requestedPage := page
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}

		out := make([]outboxEntry, 0, len(res.Tasks))
		for _, t := range res.Tasks {
			status := strings.TrimSpace(t.Status)
			if status != string(types.TaskStatusSucceeded) && status != string(types.TaskStatusFailed) && status != string(types.TaskStatusCanceled) {
				continue
			}
			out = append(out, outboxEntry{
				TaskID:       strings.TrimSpace(t.ID),
				RunID:        strings.TrimSpace(string(t.RunID)),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       status,
				Summary:      strings.TrimSpace(t.Summary),
				Error:        strings.TrimSpace(t.Error),
				InputTokens:  t.InputTokens,
				OutputTokens: t.OutputTokens,
				TotalTokens:  t.TotalTokens,
				CostUSD:      t.CostUSD,
				Timestamp:    t.CompletedAt,
			})
		}
		return outboxLoadedMsg{entries: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) scopedTaskFilter(filter agentstate.TaskFilter) agentstate.TaskFilter {
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		filter.TeamID = strings.TrimSpace(m.teamID)
		filter.RunID = strings.TrimSpace(m.focusedRunID)
		filter.AssignedRole = ""
		return filter
	}
	if strings.TrimSpace(m.teamID) != "" {
		filter.TeamID = strings.TrimSpace(m.teamID)
		filter.RunID = ""
		return filter
	}
	filter.TeamID = ""
	filter.RunID = strings.TrimSpace(m.runID)
	return filter
}

func (m *monitorModel) loadTeamStatus() tea.Cmd {
	if m == nil || m.taskStore == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	return func() tea.Msg {
		var res protocol.TeamGetStatusResult
		if err := m.rpcRoundTrip(protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			TeamID:   strings.TrimSpace(m.teamID),
		}, &res); err != nil {
			return teamStatusLoadedMsg{}
		}
		roles := make([]teamRoleState, 0, len(res.Roles))
		for _, r := range res.Roles {
			roles = append(roles, teamRoleState{Role: strings.TrimSpace(r.Role), Info: strings.TrimSpace(r.Info)})
		}
		return teamStatusLoadedMsg{
			pending:      res.Pending,
			active:       res.Active,
			done:         res.Done,
			roles:        roles,
			runIDs:       append([]string(nil), res.RunIDs...),
			roleByRunID:  res.RoleByRunID,
			totalTokens:  res.TotalTokens,
			totalCostUSD: res.TotalCostUSD,
			pricingKnown: res.PricingKnown,
		}
	}
}

func (m *monitorModel) loadTeamEvents() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	runIDs := append([]string(nil), m.teamRunIDs...)
	if len(runIDs) == 0 {
		return nil
	}
	roleByRun := map[string]string{}
	for k, v := range m.teamRoleByRunID {
		roleByRun[k] = v
	}
	cursors := map[string]int64{}
	for runID, cursor := range m.teamEventCursor {
		cursors[runID] = cursor
	}
	cfg := m.cfg
	return func() tea.Msg {
		all := make([]types.EventRecord, 0, 256)
		next := map[string]int64{}
		for _, runID := range runIDs {
			after := cursors[runID]
			batch, cursor, err := store.ListEventsPaginated(cfg, store.EventFilter{
				RunID:    runID,
				AfterSeq: after,
				Limit:    200,
				SortDesc: false,
			})
			if err != nil {
				continue
			}
			if cursor > 0 {
				next[runID] = cursor
			} else {
				next[runID] = after
			}
			role := strings.TrimSpace(roleByRun[runID])
			for _, ev := range batch {
				if ev.Data == nil {
					ev.Data = map[string]string{}
				}
				if strings.TrimSpace(ev.Data["role"]) == "" && role != "" {
					ev.Data["role"] = role
				}
				if strings.TrimSpace(ev.Data["teamId"]) == "" {
					ev.Data["teamId"] = strings.TrimSpace(m.teamID)
				}
				all = append(all, ev)
			}
		}
		sort.SliceStable(all, func(i, j int) bool {
			return all[i].Timestamp.Before(all[j].Timestamp)
		})
		return teamEventsLoadedMsg{
			events:  all,
			cursors: next,
		}
	}
}

func (m *monitorModel) loadTeamManifestCmd() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	return func() tea.Msg {
		var res protocol.TeamGetManifestResult
		err := m.rpcRoundTrip(protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			TeamID:   strings.TrimSpace(m.teamID),
		}, &res)
		if err != nil {
			if manifest, ferr := loadTeamManifestFromDisk(m.cfg, m.teamID); ferr == nil && manifest != nil {
				return teamManifestLoadedMsg{manifest: manifest, err: nil}
			}
			return teamManifestLoadedMsg{manifest: nil, err: err}
		}
		manifest := &teamManifestFile{
			TeamID:          res.TeamID,
			ProfileID:       res.ProfileID,
			TeamModel:       res.TeamModel,
			CoordinatorRole: res.CoordinatorRole,
			CoordinatorRun:  res.CoordinatorRun,
			CreatedAt:       res.CreatedAt,
		}
		if res.ModelChange != nil {
			manifest.ModelChange = &teamModelChangeFile{
				RequestedModel: res.ModelChange.RequestedModel,
				Status:         res.ModelChange.Status,
				RequestedAt:    res.ModelChange.RequestedAt,
				AppliedAt:      res.ModelChange.AppliedAt,
				Reason:         res.ModelChange.Reason,
				Error:          res.ModelChange.Error,
			}
		}
		roles := make([]teamManifestRole, 0, len(res.Roles))
		for _, r := range res.Roles {
			roles = append(roles, teamManifestRole{RoleName: r.RoleName, RunID: r.RunID, SessionID: r.SessionID})
		}
		manifest.Roles = roles
		return teamManifestLoadedMsg{manifest: manifest, err: nil}
	}
}

func (m *monitorModel) ensureFocusedRunStillValid() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.focusedRunID) == "" {
		return nil
	}
	target := strings.TrimSpace(m.focusedRunID)
	valid := false
	for _, runID := range m.teamRunIDs {
		if strings.TrimSpace(runID) == target {
			valid = true
			break
		}
	}
	if !valid {
		if _, ok := m.teamRoleByRunID[target]; ok {
			valid = true
		}
	}
	if valid {
		if role := strings.TrimSpace(m.teamRoleByRunID[target]); role != "" {
			m.focusedRunRole = role
		}
		return nil
	}
	m.focusedRunID = ""
	m.focusedRunRole = ""
	return m.applyFocusLens()
}

func (m *monitorModel) applyFocusLens() tea.Cmd {
	if m == nil {
		return nil
	}
	m.agentOutputFilteredCache = nil
	m.agentOutputLayoutWidth = 0
	m.agentOutputLineStarts = nil
	m.agentOutputLineHeights = nil
	m.agentOutputTotalLines = 0
	m.agentOutputWindowStartLine = 0
	m.dirtyAgentOutput = true
	m.dirtyActivity = true
	m.dirtyPlan = true
	m.dirtyThinking = true
	m.dirtyInbox = true
	m.dirtyOutbox = true

	return tea.Batch(
		m.loadActivityPage(),
		m.loadInboxPage(),
		m.loadOutboxPage(),
		m.loadPlanFilesCmd(),
		m.scheduleUIRefresh(),
	)
}

func (m *monitorModel) loadActivityPage() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	pageSize := m.activityPageSize
	if pageSize <= 0 {
		pageSize = 200
	}
	page := m.activityPage
	if page < 0 {
		page = 0
	}
	prevActivities := append([]Activity(nil), m.activityPageItems...)
	prevTotal := m.activityTotalCount
	prevPage := m.activityPage

	return func() tea.Msg {
		fetch := func(targetPage int) (protocol.ActivityListResult, error) {
			params := protocol.ActivityListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
				SortDesc: false,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.ActivityListResult
			if err := m.rpcRoundTrip(protocol.MethodActivityList, params, &res); err != nil {
				return protocol.ActivityListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return activityLoadedMsg{activities: []Activity{}, totalCount: 0, page: 0}
		}
		requestedPage := page
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}
		out := make([]Activity, 0, len(res.Activities))
		for _, a := range res.Activities {
			out = append(out, a)
		}
		return activityLoadedMsg{activities: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) loadPlanFiles() {
	if m == nil || m.isDetached() {
		return
	}
	params := protocol.PlanGetParams{
		ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
	}
	if strings.TrimSpace(m.teamID) != "" {
		params.TeamID = strings.TrimSpace(m.teamID)
		if strings.TrimSpace(m.focusedRunID) != "" {
			params.RunID = strings.TrimSpace(m.focusedRunID)
		} else {
			params.AggregateTeam = true
		}
	} else {
		params.RunID = strings.TrimSpace(m.runID)
	}
	var res protocol.PlanGetResult
	if err := m.rpcRoundTrip(protocol.MethodPlanGet, params, &res); err != nil {
		m.planLoadErr = err.Error()
		m.planDetailsErr = err.Error()
		m.dirtyPlan = true
		return
	}
	m.planMarkdown, m.planLoadErr = res.Checklist, res.ChecklistErr
	m.planDetails, m.planDetailsErr = res.Details, res.DetailsErr
	m.dirtyPlan = true
}

func (m *monitorModel) loadPlanFilesCmd() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	return func() tea.Msg {
		params := protocol.PlanGetParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			if strings.TrimSpace(m.focusedRunID) != "" {
				params.RunID = strings.TrimSpace(m.focusedRunID)
			} else {
				params.AggregateTeam = true
			}
		} else {
			params.RunID = strings.TrimSpace(m.runID)
		}
		var res protocol.PlanGetResult
		if err := m.rpcRoundTrip(protocol.MethodPlanGet, params, &res); err != nil {
			return planFilesLoadedMsg{checklistErr: err.Error(), detailsErr: err.Error()}
		}
		return planFilesLoadedMsg{
			checklist:    res.Checklist,
			checklistErr: res.ChecklistErr,
			details:      res.Details,
			detailsErr:   res.DetailsErr,
		}
	}
}
func (m *monitorModel) loadChildRuns() tea.Cmd {
	if m == nil || m.cfg.DataDir == "" || strings.TrimSpace(m.runID) == "" {
		return nil
	}
	cfg := m.cfg
	runID := strings.TrimSpace(m.runID)
	// If in team mode and focused on a run, show children of that run
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		runID = strings.TrimSpace(m.focusedRunID)
	}

	return func() tea.Msg {
		runs, err := store.ListChildRuns(cfg, runID)
		if err != nil {
			return childRunsLoadedMsg{err: err}
		}
		// Sort by created time
		sort.Slice(runs, func(i, j int) bool {
			if runs[i].StartedAt == nil {
				return true
			}
			if runs[j].StartedAt == nil {
				return false
			}
			return runs[i].StartedAt.Before(*runs[j].StartedAt)
		})
		return childRunsLoadedMsg{runs: runs}
	}
}
