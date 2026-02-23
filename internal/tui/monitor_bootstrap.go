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
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/tui/kit"
	agentstate "github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
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

// eventsRPCListPaginated calls events.listPaginated via RPC (for use before model exists).
func eventsRPCListPaginated(ctx context.Context, endpoint string, runID string, limit int, sortDesc bool, afterSeq int64) ([]types.EventRecord, int64, error) {
	cli := protocol.TCPClient{Endpoint: strings.TrimSpace(endpoint), Timeout: 10 * time.Second}
	params := protocol.EventsListPaginatedParams{
		RunID:    runID,
		Limit:    limit,
		SortDesc: sortDesc,
		AfterSeq: afterSeq,
	}
	var res protocol.EventsListPaginatedResult
	if err := cli.Call(ctx, protocol.MethodEventsListPaginated, params, &res); err != nil {
		return nil, 0, err
	}
	return res.Events, res.Next, nil
}

// eventsRPCLatestSeq calls events.latestSeq via RPC.
func eventsRPCLatestSeq(ctx context.Context, endpoint string, runID string) (int64, error) {
	cli := protocol.TCPClient{Endpoint: strings.TrimSpace(endpoint), Timeout: 5 * time.Second}
	params := protocol.EventsLatestSeqParams{RunID: runID}
	var res protocol.EventsLatestSeqResult
	if err := cli.Call(ctx, protocol.MethodEventsLatestSeq, params, &res); err != nil {
		return 0, err
	}
	return res.Seq, nil
}

// startEventsTailPoller runs a goroutine that polls events.listPaginated with AfterSeq and sends new events to the returned channel.
func startEventsTailPoller(ctx context.Context, endpoint string, runID string, fromSeq int64) (<-chan tailedEvent, <-chan error) {
	evCh := make(chan tailedEvent, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(evCh)
		defer close(errCh)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lastSeq := fromSeq
		failures := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				evs, next, err := eventsRPCListPaginated(ctx, endpoint, runID, 100, false, lastSeq)
				if err != nil {
					failures++
					backoff := 500 * time.Millisecond
					for i := 1; i < failures; i++ {
						backoff *= 2
						if backoff >= 8*time.Second {
							backoff = 8 * time.Second
							break
						}
					}
					ticker.Reset(backoff)
					select {
					case errCh <- err:
					default:
					}
					continue
				}
				failures = 0
				ticker.Reset(500 * time.Millisecond)
				for _, e := range evs {
					select {
					case <-ctx.Done():
						return
					case evCh <- tailedEvent{Event: e, NextOffset: next}:
					}
				}
				if next > lastSeq {
					lastSeq = next
				}
			}
		}
	}()
	return evCh, errCh
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
	endpoint := monitorRPCEndpoint()
	// Best-effort: load a small recent window via RPC.
	var evs []types.EventRecord
	if recent, _, err := eventsRPCListPaginated(ctx, endpoint, runID, 200, true, 0); err == nil {
		slices.Reverse(recent)
		evs = recent
	}

	off := int64(0)
	if seq, err := eventsRPCLatestSeq(ctx, endpoint, runID); err == nil {
		off = seq
	}
	tailCh, errCh := startEventsTailPoller(tctx, endpoint, runID, off)

	runStatus := types.RunStatusSucceeded
	runSessionID := ""
	sessionActiveModel := ""
	sessionReasoningEffort := ""
	sessionReasoningSummary := ""
	runProfile := ""
	sessionService, err := app.NewSessionServiceForCLI(cfg)
	if err != nil {
		cancel()
		return nil, err
	}

	teamID := ""
	teamRunIDs := []string{}
	teamRoleByRunID := map[string]string{}
	teamCoordinatorRunID := ""
	teamCoordinatorRole := ""
	if r, err := sessionService.LoadRun(ctx, runID); err == nil {
		runStatus = r.Status
		runSessionID = strings.TrimSpace(r.SessionID)
		if r.Runtime != nil {
			runProfile = strings.TrimSpace(r.Runtime.Profile)
			if tid := strings.TrimSpace(r.Runtime.TeamID); tid != "" {
				if manifest, err := loadTeamManifest(ctx, cfg, tid); err == nil && manifest != nil {
					teamID = tid
					teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
					teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
					for _, role := range manifest.Roles {
						rid := strings.TrimSpace(role.RunID)
						if rid == "" {
							continue
						}
						teamRunIDs = append(teamRunIDs, rid)
						teamRoleByRunID[rid] = strings.TrimSpace(role.RoleName)
					}
					if profileID := strings.TrimSpace(manifest.ProfileID); profileID != "" && runProfile == "" {
						runProfile = profileID
					}
				}
			}
		}
	}

	if runSessionID != "" {
		if sess, err := sessionService.LoadSession(ctx, runSessionID); err == nil {
			stats.totalTokensIn = sess.InputTokens
			stats.totalTokensOut = sess.OutputTokens
			stats.totalTokens = sess.TotalTokens
			stats.totalCostUSD = sess.CostUSD
			stats.pricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRunID(ctx, sessionService, runID)
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
		teamID:                      teamID,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   runStatus,
		result:                      result,
		session:                     sessionService,
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
		agentOutput:                 []AgentOutputItem{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		showThoughts:                true,
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
		seenEventIDs:                map[string]time.Time{},
		teamRunIDs:                  teamRunIDs,
		teamRoleByRunID:             teamRoleByRunID,
		teamCoordinatorRunID:        teamCoordinatorRunID,
		teamCoordinatorRole:         teamCoordinatorRole,
		teamEventCursor:             map[string]int64{},
		teamEventFailCount:          map[string]int{},
		teamEventRetryAfter:         map[string]time.Time{},
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
		if r, err := sessionService.LoadRun(ctx, runID); err == nil && r.Runtime != nil {
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
	sessionService, _ := app.NewSessionServiceForCLI(cfg)
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

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       "team:" + teamID,
		teamID:                      teamID,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionService,
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
		agentOutput:                 []AgentOutputItem{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		showThoughts:                true,
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
		seenEventIDs:                map[string]time.Time{},
		teamRunIDs:                  teamRunIDs,
		teamRoleByRunID:             teamRoleByRun,
		teamCoordinatorRunID:        teamCoordinatorRunID,
		teamCoordinatorRole:         teamCoordinatorRole,
		teamEventCursor:             teamEventCursor,
		teamEventFailCount:          map[string]int{},
		teamEventRetryAfter:         map[string]time.Time{},
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
	if manifest, err := loadTeamManifest(ctx, cfg, teamID); err == nil && manifest != nil {
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

	sessionService, err := app.NewSessionServiceForCLI(cfg)
	if err != nil {
		return nil, err
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionService,
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
		agentOutput:                 []AgentOutputItem{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		showThoughts:                true,
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
		seenEventIDs:                map[string]time.Time{},
		teamRoleByRunID:             map[string]string{},
		teamEventCursor:             map[string]int64{},
		teamEventFailCount:          map[string]int{},
		teamEventRetryAfter:         map[string]time.Time{},
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
