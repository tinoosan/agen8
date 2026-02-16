package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func RunMonitor(ctx context.Context, cfg config.Config, runID string) error {
	if err := ensureRPCReachable(ctx); err != nil {
		return err
	}
	var result MonitorResult
	m, err := newMonitorModel(ctx, cfg, runID, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
	return err
}

func RunMonitorDetached(ctx context.Context, cfg config.Config) error {
	var result MonitorResult
	m, err := newDetachedMonitorModel(ctx, cfg, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
	return err
}

func RunTeamMonitor(ctx context.Context, cfg config.Config, teamID string) error {
	if err := ensureRPCReachable(ctx); err != nil {
		return err
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return fmt.Errorf("teamID is required")
	}
	var result MonitorResult
	m, err := newTeamMonitorModel(ctx, cfg, teamID, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
	return err
}

func newMonitorModel(ctx context.Context, cfg config.Config, runID string, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	stats := monitorStats{started: time.Now()}

	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return nil, err
	}
	if rs, err := taskStore.GetRunStats(ctx, runID); err == nil {
		// Best-effort: show tasks already completed before monitor attached.
		stats.tasksDone = rs.Succeeded + rs.Failed
	}

	in := textarea.New()
	in.SetHeight(2)
	in.CharLimit = 0
	in.Placeholder = "Type a task or command..."
	in.ShowLineNumbers = false
	in.FocusedStyle.CursorLine = lipgloss.NewStyle()
	in.FocusedStyle.Placeholder = kit.StyleDim
	in.FocusedStyle.Text = kit.StyleStatusValue
	in.FocusedStyle.Prompt = kit.StyleStatusKey
	in.Prompt = ""
	in.Focus()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 0, 0)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	tctx, cancel := context.WithCancel(ctx)
	// Best-effort: load a small recent window for initial display without scanning the full log.
	var evs []types.EventRecord
	{
		filter := store.EventFilter{
			RunID:    runID,
			Limit:    200,
			SortDesc: true, // newest first
		}
		if recent, _, err := store.ListEventsPaginated(cfg, filter); err == nil {
			// Observe in chronological order (oldest -> newest) to preserve ordering semantics.
			slices.Reverse(recent)
			evs = recent
		}
	}

	off := int64(0)
	if latest, err := store.GetLatestEventSeq(cfg, runID); err == nil {
		off = latest
	}
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	runStatus := types.RunStatusSucceeded
	runSessionID := ""
	sessionActiveModel := ""
	sessionReasoningEffort := ""
	sessionReasoningSummary := ""
	runProfile := ""
	if r, err := store.LoadRun(cfg, runID); err == nil {
		runStatus = r.Status
		runSessionID = strings.TrimSpace(r.SessionID)
		if r.Runtime != nil {
			runProfile = strings.TrimSpace(r.Runtime.Profile)
		}
	}

	sessionStore, err := store.NewSQLiteSessionStore(cfg)
	if err != nil {
		cancel()
		return nil, err
	}

	if runSessionID != "" {
		if sess, err := sessionStore.LoadSession(ctx, runSessionID); err == nil {
			stats.totalTokensIn = sess.InputTokens
			stats.totalTokensOut = sess.OutputTokens
			stats.totalTokens = sess.TotalTokens
			stats.totalCostUSD = sess.CostUSD
			stats.pricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRunID(cfg, runID)
			if active := strings.TrimSpace(sess.ActiveModel); active != "" {
				sessionActiveModel = active
			}
			sessionReasoningEffort = strings.TrimSpace(sess.ReasoningEffort)
			sessionReasoningSummary = strings.TrimSpace(sess.ReasoningSummary)
		}
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       runID,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   runStatus,
		result:                      result,
		session:                     sessionStore,
		sessionID:                   runSessionID,
		offset:                      off,
		input:                       in,
		activityPageItems:           []Activity{},
		activityPage:                0,
		activityPageSize:            200,
		activityTotalCount:          0,
		activityList:                activityList,
		activityDetail:              viewport.New(0, 0),
		activityFollowingTail:       true,
		planViewport:                viewport.New(0, 0),
		planFollowingTop:            true,
		renderer:                    newContentRenderer(),
		agentOutput:                 []string{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		inbox:                       map[string]taskState{},
		inboxVP:                     viewport.New(0, 0),
		inboxList:                   []taskState{},
		inboxPage:                   0,
		inboxPageSize:               50,
		outboxResults:               []outboxEntry{},
		outboxVP:                    viewport.New(0, 0),
		outboxPage:                  0,
		outboxPageSize:              50,
		memResults:                  []string{},
		memoryVP:                    viewport.New(0, 0),
		thinkingEntries:             []thinkingEntry{},
		reasoningUsageByStep:        map[string]int{},
		thinkingVP:                  viewport.New(0, 0),
		thinkingAutoScroll:          true,
		subagentsVP:                 viewport.New(0, 0),
		subagentsList:               newSubagentsList(),
		artifactContentVP:           viewport.New(0, 0),
		taskStore:                   taskStore,
		stats:                       stats,
		styles:                      defaultMonitorStyles(),
		focusedPanel:                panelComposer,
		tailCh:                      tailCh,
		errCh:                       errCh,
		cancel:                      cancel,
		uiRefreshDebounce:           33 * time.Millisecond,
		planReloadDebounce:          100 * time.Millisecond,
		sessionTotalsReloadDebounce: 150 * time.Millisecond,
		seenOutboxByTask:            map[string]struct{}{},
		teamRoleByRunID:             map[string]string{},
		teamEventCursor:             map[string]int64{},
	}
	if sessionActiveModel != "" {
		m.model = sessionActiveModel
	}
	if sessionReasoningEffort != "" {
		m.reasoningEffort = sessionReasoningEffort
	}
	if sessionReasoningSummary != "" {
		m.reasoningSummary = sessionReasoningSummary
	}
	// Consolidate model/profile initialization:
	// 1. Profile: Run's runtime profile (primary)
	// 2. Model: Session's ActiveModel (primary) > Run's Runtime Model > Profile default
	if runProfile != "" {
		m.profile = runProfile
	}

	// Replay events to build up state (e.g. inbox, activity)
	for _, e := range evs {
		m.observeEvent(e)
	}

	// Enforce session state as the source of truth for the model, overriding any
	// transient state from event replay.
	if sessionActiveModel != "" {
		m.model = sessionActiveModel
	} else if m.model == "" {
		// Fallback order if session didn't have an active model (unlikely for active sessions):
		// 1. Runtime model (from run record)
		// 2. "default" (will display as default)
		if r, err := store.LoadRun(cfg, runID); err == nil && r.Runtime != nil {
			if m.model == "" {
				m.model = strings.TrimSpace(r.Runtime.Model)
			}
		}
	}
	if m.profile == "" {
		m.profile = "default"
	}
	if m.model == "" {
		m.model = "default"
	}
	// Activity feed is loaded from SQLite (paginated) via loadActivityPage.
	m.loadPlanFiles()
	m.refreshViewports()

	return m, nil
}

func newTeamMonitorModel(ctx context.Context, cfg config.Config, teamID string, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("teamID is required")
	}
	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return nil, err
	}
	sessionStore, _ := store.NewSQLiteSessionStore(cfg)
	in := textarea.New()
	in.SetHeight(2)
	in.CharLimit = 0
	in.Placeholder = "Type a task or command..."
	in.ShowLineNumbers = false
	in.FocusedStyle.CursorLine = lipgloss.NewStyle()
	in.FocusedStyle.Placeholder = kit.StyleDim
	in.FocusedStyle.Text = kit.StyleStatusValue
	in.FocusedStyle.Prompt = kit.StyleStatusKey
	in.Prompt = ""
	in.Focus()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 0, 0)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)
	teamRoleByRun := map[string]string{}
	teamRunIDs := []string{}
	teamCoordinatorRunID := ""
	teamCoordinatorRole := ""
	teamEventCursor := map[string]int64{}
	for _, runID := range teamRunIDs {
		if latest, err := store.GetLatestEventSeq(cfg, runID); err == nil {
			start := latest - 150
			if start < 0 {
				start = 0
			}
			teamEventCursor[runID] = start
		} else {
			teamEventCursor[runID] = 0
		}
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       "team:" + teamID,
		teamID:                      teamID,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionStore,
		sessionID:                   "",
		offset:                      0,
		input:                       in,
		activityPageItems:           []Activity{},
		activityPage:                0,
		activityPageSize:            200,
		activityTotalCount:          0,
		activityList:                activityList,
		activityDetail:              viewport.New(0, 0),
		activityFollowingTail:       true,
		planViewport:                viewport.New(0, 0),
		planFollowingTop:            true,
		renderer:                    newContentRenderer(),
		agentOutput:                 []string{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		inbox:                       map[string]taskState{},
		inboxVP:                     viewport.New(0, 0),
		inboxList:                   []taskState{},
		inboxPage:                   0,
		inboxPageSize:               50,
		outboxResults:               []outboxEntry{},
		outboxVP:                    viewport.New(0, 0),
		outboxPage:                  0,
		outboxPageSize:              50,
		memResults:                  []string{},
		memoryVP:                    viewport.New(0, 0),
		thinkingEntries:             []thinkingEntry{},
		reasoningUsageByStep:        map[string]int{},
		thinkingVP:                  viewport.New(0, 0),
		thinkingAutoScroll:          true,
		subagentsVP:                 viewport.New(0, 0),
		subagentsList:               newSubagentsList(),
		artifactContentVP:           viewport.New(0, 0),
		taskStore:                   taskStore,
		stats:                       monitorStats{started: time.Now()},
		styles:                      defaultMonitorStyles(),
		focusedPanel:                panelComposer,
		uiRefreshDebounce:           33 * time.Millisecond,
		planReloadDebounce:          100 * time.Millisecond,
		sessionTotalsReloadDebounce: 150 * time.Millisecond,
		seenOutboxByTask:            map[string]struct{}{},
		teamRunIDs:                  teamRunIDs,
		teamRoleByRunID:             teamRoleByRun,
		teamCoordinatorRunID:        teamCoordinatorRunID,
		teamCoordinatorRole:         teamCoordinatorRole,
		teamEventCursor:             teamEventCursor,
	}
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false
	m.subagentsVP.MouseWheelEnabled = false
	m.artifactContentVP.MouseWheelEnabled = false
	if manifest, err := loadTeamManifestFromDisk(cfg, teamID); err == nil && manifest != nil {
		if profileID := strings.TrimSpace(manifest.ProfileID); profileID != "" {
			m.profile = profileID
		}
		if mc := manifest.ModelChange; mc != nil {
			if requested := strings.TrimSpace(mc.RequestedModel); requested != "" &&
				(strings.EqualFold(strings.TrimSpace(mc.Status), "pending") ||
					strings.EqualFold(strings.TrimSpace(mc.Status), "applied")) {
				m.model = requested
			}
		}
		if strings.TrimSpace(m.model) == "" {
			if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
				m.model = teamModel
			}
		}
		m.teamModelChange = manifest.ModelChange
		m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
		m.sessionID = resolveTeamControlSessionID(manifest, m.sessionID)
		roleByRun := map[string]string{}
		runIDs := make([]string, 0, len(manifest.Roles))
		for _, role := range manifest.Roles {
			runID := strings.TrimSpace(role.RunID)
			if runID == "" {
				continue
			}
			roleByRun[runID] = strings.TrimSpace(role.RoleName)
			runIDs = append(runIDs, runID)
		}
		if len(runIDs) != 0 {
			m.teamRunIDs = runIDs
			m.teamRoleByRunID = roleByRun
		}
	}
	if m.session != nil && strings.TrimSpace(m.sessionID) != "" {
		if sess, err := m.session.LoadSession(ctx, strings.TrimSpace(m.sessionID)); err == nil {
			if v := strings.TrimSpace(sess.ReasoningEffort); v != "" {
				m.reasoningEffort = v
			}
			if v := strings.TrimSpace(sess.ReasoningSummary); v != "" {
				m.reasoningSummary = v
			}
			// Enforce session state as the source of truth for the model, overriding manifest
			if v := strings.TrimSpace(sess.ActiveModel); v != "" {
				m.model = v
			}
		}
	}
	if m.profile == "" {
		m.profile = "default"
	}
	if m.model == "" {
		m.model = "default"
	}
	m.refreshViewports()
	return m, nil
}

func newDetachedMonitorModel(ctx context.Context, cfg config.Config, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return nil, err
	}
	in := textarea.New()
	in.SetHeight(2)
	in.CharLimit = 0
	in.Placeholder = "Type a task or command..."
	in.ShowLineNumbers = false
	in.FocusedStyle.CursorLine = lipgloss.NewStyle()
	in.FocusedStyle.Placeholder = kit.StyleDim
	in.FocusedStyle.Text = kit.StyleStatusValue
	in.FocusedStyle.Prompt = kit.StyleStatusKey
	in.Prompt = ""
	in.Focus()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 0, 0)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	sessionStore, err := store.NewSQLiteSessionStore(cfg)
	if err != nil {
		return nil, err
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionStore,
		sessionID:                   "",
		input:                       in,
		activityPageItems:           []Activity{},
		activityPage:                0,
		activityPageSize:            200,
		activityTotalCount:          0,
		activityList:                activityList,
		activityDetail:              viewport.New(0, 0),
		activityFollowingTail:       true,
		planViewport:                viewport.New(0, 0),
		planFollowingTop:            true,
		renderer:                    newContentRenderer(),
		agentOutput:                 []string{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		inbox:                       map[string]taskState{},
		inboxVP:                     viewport.New(0, 0),
		inboxList:                   []taskState{},
		inboxPage:                   0,
		inboxPageSize:               50,
		outboxResults:               []outboxEntry{},
		outboxVP:                    viewport.New(0, 0),
		outboxPage:                  0,
		outboxPageSize:              50,
		memResults:                  []string{},
		memoryVP:                    viewport.New(0, 0),
		thinkingEntries:             []thinkingEntry{},
		reasoningUsageByStep:        map[string]int{},
		thinkingVP:                  viewport.New(0, 0),
		thinkingAutoScroll:          true,
		subagentsVP:                 viewport.New(0, 0),
		subagentsList:               newSubagentsList(),
		artifactContentVP:           viewport.New(0, 0),
		taskStore:                   taskStore,
		stats:                       monitorStats{started: time.Now()},
		styles:                      defaultMonitorStyles(),
		focusedPanel:                panelComposer,
		uiRefreshDebounce:           33 * time.Millisecond,
		planReloadDebounce:          100 * time.Millisecond,
		sessionTotalsReloadDebounce: 150 * time.Millisecond,
		seenOutboxByTask:            map[string]struct{}{},
		teamRoleByRunID:             map[string]string{},
		teamEventCursor:             map[string]int64{},
		detached:                    true,
	}
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false
	m.subagentsVP.MouseWheelEnabled = false
	m.artifactContentVP.MouseWheelEnabled = false
	m.appendAgentOutput("[system] No active context. Use /new, /sessions, or /agents.")
	m.refreshViewports()
	return m, nil
}

// loadPendingTasksFromSQLite queries pending tasks for the run. Used so the queue
// shows tasks added before the monitor started or via webhook, without scanning
// inbox files.
