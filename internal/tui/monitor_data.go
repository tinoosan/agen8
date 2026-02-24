package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	agentstate "github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

func pricingKnownForRunID(ctx context.Context, session pkgsession.Service, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || session == nil {
		return false
	}
	run, err := session.LoadRun(ctx, runID)
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

func loadTeamManifest(ctx context.Context, cfg config.Config, teamID string) (*teamManifestFile, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, nil
	}
	store := team.NewFileManifestStore(cfg)
	m, err := store.Load(ctx, teamID)
	if err != nil || m == nil {
		return nil, err
	}
	return manifestToFile(m), nil
}

func manifestToFile(m *team.Manifest) *teamManifestFile {
	if m == nil {
		return nil
	}
	roles := make([]teamManifestRole, len(m.Roles))
	for i, r := range m.Roles {
		roles[i] = teamManifestRole{RoleName: r.RoleName, RunID: r.RunID, SessionID: r.SessionID}
	}
	var modelChange *teamModelChangeFile
	if m.ModelChange != nil {
		modelChange = &teamModelChangeFile{
			RequestedModel: m.ModelChange.RequestedModel,
			Status:         m.ModelChange.Status,
			RequestedAt:    m.ModelChange.RequestedAt,
			AppliedAt:      m.ModelChange.AppliedAt,
			Reason:         m.ModelChange.Reason,
			Error:          m.ModelChange.Error,
		}
	}
	return &teamManifestFile{
		TeamID:          m.TeamID,
		ProfileID:       m.ProfileID,
		TeamModel:       m.TeamModel,
		ModelChange:     modelChange,
		CoordinatorRole: m.CoordinatorRole,
		CoordinatorRun:  m.CoordinatorRun,
		Roles:           roles,
		CreatedAt:       m.CreatedAt,
	}
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
		if manifest, err := loadTeamManifest(m.ctx, m.cfg, m.teamID); err == nil && manifest != nil {
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
	teamID := strings.TrimSpace(m.teamID)
	if teamID == "" {
		return strings.TrimSpace(m.sessionID)
	}
	sessionID := ""
	if manifest, err := loadTeamManifest(m.ctx, m.cfg, teamID); err == nil && manifest != nil {
		sessionID = resolveTeamControlSessionID(manifest, "")
		if sessionID != "" {
			m.sessionID = sessionID
		}
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.sessionID)
	}
	return strings.TrimSpace(sessionID)
}

func (m *monitorModel) resolveFreshTeamControlSessionID(ctx context.Context) (string, error) {
	if m == nil {
		return "", fmt.Errorf("%w: monitor is nil", rpcscope.ErrScopeUnavailable)
	}
	teamID := strings.TrimSpace(m.teamID)
	preferred := strings.TrimSpace(m.resolveTeamControlSessionID())
	if preferred == "" {
		preferred = strings.TrimSpace(m.sessionID)
	}
	endpoint := strings.TrimSpace(m.rpcEndpoint)
	if endpoint == "" {
		endpoint = monitorRPCEndpoint()
	}

	sessionID, err := rpcscope.ResolveControlSessionID(ctx, endpoint, preferred, teamID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("%w: team control session unavailable", rpcscope.ErrScopeUnavailable)
	}
	m.sessionID = strings.TrimSpace(sessionID)
	return strings.TrimSpace(sessionID), nil
}

func (m *monitorModel) resolveEnqueueTargetRunID() (string, error) {
	if m == nil {
		return "", fmt.Errorf("monitor is nil")
	}
	if strings.TrimSpace(m.teamID) == "" {
		runID := strings.TrimSpace(m.runID)
		if runID == "" {
			return "", fmt.Errorf("active run is unavailable")
		}
		return runID, nil
	}
	if runID := strings.TrimSpace(m.focusedRunID); runID != "" {
		return runID, nil
	}
	if runID := strings.TrimSpace(m.teamCoordinatorRunID); runID != "" {
		return runID, nil
	}
	manifest, err := loadTeamManifest(m.ctx, m.cfg, strings.TrimSpace(m.teamID))
	if err != nil || manifest == nil {
		return "", fmt.Errorf("coordinator run unavailable")
	}
	m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
	m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
	if m.teamRoleByRunID == nil {
		m.teamRoleByRunID = map[string]string{}
	}
	for _, role := range manifest.Roles {
		runID := strings.TrimSpace(role.RunID)
		if runID == "" {
			continue
		}
		m.teamRoleByRunID[runID] = strings.TrimSpace(role.RoleName)
	}
	if sessionID := strings.TrimSpace(resolveTeamControlSessionID(manifest, m.sessionID)); sessionID != "" {
		m.sessionID = sessionID
	}
	if strings.TrimSpace(m.teamCoordinatorRunID) == "" {
		return "", fmt.Errorf("coordinator run unavailable")
	}
	return strings.TrimSpace(m.teamCoordinatorRunID), nil
}

func (m *monitorModel) resolveSessionIDForRun(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return strings.TrimSpace(m.rpcRun().SessionID)
	}
	if strings.TrimSpace(m.teamID) == "" {
		return strings.TrimSpace(m.rpcRun().SessionID)
	}
	if manifest, err := loadTeamManifest(m.ctx, m.cfg, strings.TrimSpace(m.teamID)); err == nil && manifest != nil {
		for _, role := range manifest.Roles {
			if strings.TrimSpace(role.RunID) == runID {
				sessionID := strings.TrimSpace(role.SessionID)
				if sessionID != "" {
					return sessionID
				}
			}
		}
	}
	return strings.TrimSpace(m.rpcRun().SessionID)
}

func (m *monitorModel) resolveRoleForRun(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	if role := strings.TrimSpace(m.teamRoleByRunID[runID]); role != "" {
		return role
	}
	if strings.TrimSpace(m.teamID) == "" {
		return ""
	}
	manifest, err := loadTeamManifest(m.ctx, m.cfg, strings.TrimSpace(m.teamID))
	if err != nil || manifest == nil {
		return ""
	}
	if m.teamRoleByRunID == nil {
		m.teamRoleByRunID = map[string]string{}
	}
	for _, entry := range manifest.Roles {
		rid := strings.TrimSpace(entry.RunID)
		if rid == "" {
			continue
		}
		m.teamRoleByRunID[rid] = strings.TrimSpace(entry.RoleName)
	}
	return strings.TrimSpace(m.teamRoleByRunID[runID])
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
	v := strings.TrimSpace(os.Getenv("AGEN8_RPC_ENDPOINT"))
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
	if m.session != nil && strings.TrimSpace(m.sessionID) != "" {
		if sess, serr := m.session.LoadSession(ctx, strings.TrimSpace(m.sessionID)); serr == nil {
			sess.ActiveModel = model
			_ = m.session.SaveSession(ctx, sess)
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
		} else {
			// Session Stats should always come from session totals, not a run override.
			params.RunID = ""
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
			var res protocol.TaskListResult
			client := rpcscope.NewClient(strings.TrimSpace(m.rpcEndpoint), strings.TrimSpace(m.rpcRun().SessionID)).WithTimeout(2 * time.Second)
			client.SetState(rpcscope.ScopeState{
				SessionID: strings.TrimSpace(m.rpcRun().SessionID),
				ThreadID:  strings.TrimSpace(m.rpcRun().SessionID),
				TeamID:    strings.TrimSpace(m.teamID),
				RunID:     strings.TrimSpace(m.runID),
			})
			_, _, err := client.CallWithRecovery(m.ctx, protocol.MethodTaskList, func(scope rpcscope.ScopeState) (any, error) {
				params := protocol.TaskListParams{
					ThreadID: protocol.ThreadID(scope.ThreadID),
					TeamID:   strings.TrimSpace(scope.TeamID),
					RunID:    strings.TrimSpace(scope.RunID),
					View:     "inbox",
					Limit:    pageSize,
					Offset:   targetPage * pageSize,
				}
				// In team overview, use team scope (RunID="") so inbox includes tasks
				// assigned to any role/run, including subagents.
				if strings.TrimSpace(scope.TeamID) != "" && (params.RunID == "" || strings.HasPrefix(strings.ToLower(params.RunID), "team:")) {
					params.RunID = ""
				}
				if strings.TrimSpace(scope.TeamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
				return params, nil
			}, &res)
			if err != nil {
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
			if status != string(types.TaskStatusPending) &&
				status != string(types.TaskStatusActive) &&
				status != string(types.TaskStatusReviewPending) {
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
			var res protocol.TaskListResult
			client := rpcscope.NewClient(strings.TrimSpace(m.rpcEndpoint), strings.TrimSpace(m.rpcRun().SessionID)).WithTimeout(2 * time.Second)
			client.SetState(rpcscope.ScopeState{
				SessionID: strings.TrimSpace(m.rpcRun().SessionID),
				ThreadID:  strings.TrimSpace(m.rpcRun().SessionID),
				TeamID:    strings.TrimSpace(m.teamID),
				RunID:     strings.TrimSpace(m.runID),
			})
			_, _, err := client.CallWithRecovery(m.ctx, protocol.MethodTaskList, func(scope rpcscope.ScopeState) (any, error) {
				params := protocol.TaskListParams{
					ThreadID: protocol.ThreadID(scope.ThreadID),
					TeamID:   strings.TrimSpace(scope.TeamID),
					RunID:    strings.TrimSpace(scope.RunID),
					View:     "outbox",
					Limit:    pageSize,
					Offset:   targetPage * pageSize,
				}
				// In team overview, use team scope (RunID="") so outbox includes tasks
				// completed by any role/run, including subagents.
				if strings.TrimSpace(scope.TeamID) != "" && (params.RunID == "" || strings.HasPrefix(strings.ToLower(params.RunID), "team:")) {
					params.RunID = ""
				}
				if strings.TrimSpace(scope.TeamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
				return params, nil
			}, &res)
			if err != nil {
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
			pending:        res.Pending,
			active:         res.Active,
			done:           res.Done,
			roles:          roles,
			runIDs:         append([]string(nil), res.RunIDs...),
			roleByRunID:    res.RoleByRunID,
			totalTokensIn:  res.TotalTokensIn,
			totalTokensOut: res.TotalTokensOut,
			totalTokens:    res.TotalTokens,
			totalCostUSD:   res.TotalCostUSD,
			pricingKnown:   res.PricingKnown,
		}
	}
}

func (m *monitorModel) loadTeamEvents() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	roleByRun := map[string]string{}
	for k, v := range m.teamRoleByRunID {
		roleByRun[k] = v
	}
	runIDs := mergeTeamAndChildRunIDs(m.teamRunIDs, m.childRuns, roleByRun)
	if len(runIDs) == 0 {
		return nil
	}
	cursors := map[string]int64{}
	for runID, cursor := range m.teamEventCursor {
		cursors[runID] = cursor
	}
	failCounts := map[string]int{}
	for runID, count := range m.teamEventFailCount {
		failCounts[runID] = count
	}
	retryAfter := map[string]time.Time{}
	for runID, at := range m.teamEventRetryAfter {
		retryAfter[runID] = at
	}
	return func() tea.Msg {
		all := make([]types.EventRecord, 0, 256)
		next := map[string]int64{}
		nextFail := map[string]int{}
		nextRetry := map[string]time.Time{}
		now := time.Now()
		for _, runID := range runIDs {
			if at := retryAfter[runID]; !at.IsZero() && now.Before(at) {
				nextFail[runID] = failCounts[runID]
				nextRetry[runID] = at
				next[runID] = cursors[runID]
				continue
			}
			after := cursors[runID]
			var res protocol.EventsListPaginatedResult
			if err := m.rpcRoundTrip(protocol.MethodEventsListPaginated, protocol.EventsListPaginatedParams{
				RunID:    runID,
				AfterSeq: after,
				Limit:    200,
				SortDesc: false,
			}, &res); err != nil {
				fail := failCounts[runID] + 1
				nextFail[runID] = fail
				backoff := 500 * time.Millisecond
				for i := 1; i < fail; i++ {
					backoff *= 2
					if backoff >= 8*time.Second {
						backoff = 8 * time.Second
						break
					}
				}
				nextRetry[runID] = now.Add(backoff)
				next[runID] = after
				continue
			}
			nextFail[runID] = 0
			batch := res.Events
			cursor := res.Next
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
			failed:  nextFail,
			retryAt: nextRetry,
		}
	}
}

func mergeTeamAndChildRunIDs(teamRunIDs []string, childRuns []types.Run, roleByRun map[string]string) []string {
	merged := make([]string, 0, len(teamRunIDs)+len(childRuns))
	seen := map[string]struct{}{}
	for _, runID := range teamRunIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		merged = append(merged, runID)
	}
	for _, run := range childRuns {
		runID := strings.TrimSpace(run.RunID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; !ok {
			seen[runID] = struct{}{}
			merged = append(merged, runID)
		}
		if roleByRun != nil && strings.TrimSpace(roleByRun[runID]) == "" {
			spawnIndex := run.SpawnIndex
			if spawnIndex <= 0 {
				spawnIndex = 1
			}
			roleByRun[runID] = fmt.Sprintf("Subagent-%d", spawnIndex)
		}
	}
	return merged
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
			if manifest, ferr := loadTeamManifest(m.ctx, m.cfg, m.teamID); ferr == nil && manifest != nil {
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
				params.IncludeChildRuns = true // Show tasks from sub-agents in the same activity list
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

// agentListItemToRun maps a protocol.AgentListItem to a types.Run for Subagents tab display.
func agentListItemToRun(it protocol.AgentListItem, spawnIndex int) types.Run {
	r := types.Run{
		RunID:       strings.TrimSpace(it.RunID),
		SessionID:   strings.TrimSpace(it.SessionID),
		Goal:        strings.TrimSpace(it.Goal),
		Status:      strings.TrimSpace(it.Status),
		ParentRunID: strings.TrimSpace(it.ParentRunID),
		SpawnIndex:  spawnIndex,
	}
	if s := strings.TrimSpace(it.StartedAt); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			r.StartedAt = &t
		}
	}
	if s := strings.TrimSpace(it.FinishedAt); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			r.FinishedAt = &t
		}
	}
	return r
}

func (m *monitorModel) loadChildRuns() tea.Cmd {
	if m == nil || strings.TrimSpace(m.runID) == "" {
		return nil
	}
	runID := strings.TrimSpace(m.runID)
	// If in team mode and focused on a run, show children of that run
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		runID = strings.TrimSpace(m.focusedRunID)
	}

	return func() tea.Msg {
		parentRunIDs := []string{}
		isTeamOverview := strings.TrimSpace(m.teamID) != "" &&
			strings.TrimSpace(m.focusedRunID) == "" &&
			(runID == "" || strings.HasPrefix(strings.ToLower(runID), "team:"))
		if isTeamOverview && len(m.teamRunIDs) > 0 {
			seenParents := map[string]struct{}{}
			for _, id := range m.teamRunIDs {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if _, ok := seenParents[id]; ok {
					continue
				}
				seenParents[id] = struct{}{}
				parentRunIDs = append(parentRunIDs, id)
			}
		}
		if len(parentRunIDs) == 0 {
			parentRunIDs = append(parentRunIDs, runID)
		}

		merged := make([]types.Run, 0, 8)
		seenChild := map[string]struct{}{}
		for _, parentID := range parentRunIDs {
			parentID = strings.TrimSpace(parentID)
			if parentID == "" || strings.HasPrefix(strings.ToLower(parentID), "team:") {
				continue
			}
			var childRes protocol.RunListChildrenResult
			if err := m.rpcRoundTrip(protocol.MethodRunListChildren, protocol.RunListChildrenParams{ParentRunID: parentID}, &childRes); err != nil {
				return childRunsLoadedMsg{err: err}
			}
			for _, run := range childRes.Runs {
				rid := strings.TrimSpace(run.RunID)
				if rid == "" {
					continue
				}
				if _, ok := seenChild[rid]; ok {
					continue
				}
				seenChild[rid] = struct{}{}
				merged = append(merged, run)
			}
		}

		runs := merged
		sort.Slice(runs, func(i, j int) bool {
			if runs[i].StartedAt == nil {
				return true
			}
			if runs[j].StartedAt == nil {
				return false
			}
			return runs[i].StartedAt.Before(*runs[j].StartedAt)
		})
		assignedByRunID := map[string]int{}
		completedByRunID := map[string]int{}
		activeByRunID := map[string]int{}
		if m.taskStore != nil {
			for _, run := range runs {
				rid := strings.TrimSpace(run.RunID)
				if rid == "" {
					continue
				}
				if n, err := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{RunID: rid}); err == nil {
					assignedByRunID[rid] = n
				}
				if n, err := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{
					RunID: rid,
					Status: []types.TaskStatus{
						types.TaskStatusSucceeded,
						types.TaskStatusFailed,
						types.TaskStatusCanceled,
					},
				}); err == nil {
					completedByRunID[rid] = n
				}
				if n, err := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{
					RunID:  rid,
					Status: []types.TaskStatus{types.TaskStatusActive},
				}); err == nil {
					activeByRunID[rid] = n
				}
			}
			callbackPendingBySourceRunID := listReviewPendingCallbacksBySourceRunID(m.ctx, m.taskStore, strings.TrimSpace(m.teamID), strings.TrimSpace(m.rpcRun().SessionID))
			for rid, pending := range callbackPendingBySourceRunID {
				if pending <= 0 {
					continue
				}
				// Keep callback review gates visible as queued work on the source subagent row.
				assignedByRunID[rid] += pending
			}
		}
		return childRunsLoadedMsg{
			runs:             runs,
			assignedByRunID:  assignedByRunID,
			completedByRunID: completedByRunID,
			activeByRunID:    activeByRunID,
		}
	}
}

func listReviewPendingCallbacksBySourceRunID(ctx context.Context, store agentstate.TaskStore, teamID, sessionID string) map[string]int {
	out := map[string]int{}
	if store == nil {
		return out
	}
	filter := agentstate.TaskFilter{
		Status: []types.TaskStatus{types.TaskStatusReviewPending},
		Limit:  1000,
		SortBy: "created_at",
	}
	if strings.TrimSpace(teamID) != "" {
		filter.TeamID = strings.TrimSpace(teamID)
	} else {
		filter.SessionID = strings.TrimSpace(sessionID)
	}
	tasks, err := store.ListTasks(ctx, filter)
	if err != nil {
		return out
	}
	for _, task := range tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}
		if len(task.Metadata) == 0 {
			if loaded, lerr := store.GetTask(ctx, taskID); lerr == nil {
				task = loaded
			}
		}
		source := strings.TrimSpace(metadataStringAny(task.Metadata, "source"))
		if !strings.Contains(strings.ToLower(source), "callback") {
			continue
		}
		sourceRunID := strings.TrimSpace(metadataStringAny(task.Metadata, "sourceRunId"))
		if sourceRunID == "" {
			sourceRunID = strings.TrimSpace(metadataStringAny(task.Metadata, "sourceRunID"))
		}
		if sourceRunID == "" {
			continue
		}
		out[sourceRunID]++
	}
	return out
}

func metadataStringAny(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}
