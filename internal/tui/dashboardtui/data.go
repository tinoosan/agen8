package dashboardtui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	"github.com/tinoosan/agen8/internal/tui/sessionsync"
	"github.com/tinoosan/agen8/pkg/protocol"
)

const detachedThreadID = protocol.ThreadID("detached-control")

type teamRow struct {
	TeamID           string
	ProfileID        string
	PrimarySessionID string
	CoordinatorRunID string
	Status           string
	CreatedAt        string
	UpdatedAt        string
	ManifestPresent  bool

	// Enriched from TeamGetStatus, TeamGetManifest, RuntimeGetSessionState.
	Pending           int
	Active            int
	Done              int
	TotalTokens       int
	TotalCostUSD      float64
	RunningAgents     int
	TotalAgents       int
	CoordinatorRole   string
	CoordinatorStatus string
	HasBlockedTasks   bool
}

type agentRow struct {
	Role            string
	RunID           string
	Status          string
	Profile         string
	Model           string
	RunTotalTokens  int
	RunTotalCostUSD float64
	AssignedTasks   int
	CompletedTasks  int
	WorkerPresent   bool
	StartedAt       string
}

type sessionStats struct {
	TotalTokens  int
	TotalCostUSD float64
	Assigned     int
	Completed    int
	Pending      int
	Active       int
	Done         int
	RunningCount int
}

type dataLoadedMsg struct {
	agents       []agentRow
	stats        sessionStats
	sessionMode  string
	teamID       string
	runID        string
	reviewerRole string
	preserve     bool
	connected    bool
	err          error
}

type projectDataLoadedMsg struct {
	teams     []teamRow
	projectID string
	err       error
}

type tickMsg struct{}

type sessionSyncedMsg struct {
	sessionID string
	changed   bool
	err       error
}

type dashboardTaskCounts struct {
	AssignedByRun   map[string]int
	CompletedByRun  map[string]int
	AssignedByRole  map[string]int
	CompletedByRole map[string]int
	Pending         int
	Active          int
	Done            int
	Assigned        int
	Completed       int
}

func fetchDataCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cli := protocol.TCPClient{
			Endpoint: endpoint,
			Timeout:  5 * time.Second,
		}

		call := func(method string, params, out any) error {
			if err := cli.Call(ctx, method, params, out); err != nil {
				return fmt.Errorf("rpc %s: %w", method, err)
			}
			return nil
		}

		sid := strings.TrimSpace(sessionID)
		if sid == "" {
			return dataLoadedMsg{err: fmt.Errorf("session id is required")}
		}

		scopeClient := rpcscope.NewClient(endpoint, sid).WithTimeout(5 * time.Second)
		scope, err := scopeClient.RefreshScope(ctx)
		if err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}
		mode := kit.Fallback(strings.TrimSpace(scope.Mode), "team")
		teamID := strings.TrimSpace(scope.TeamID)
		runID := strings.TrimSpace(scope.RunID)
		reviewerRole := ""
		threadID := protocol.ThreadID(scope.ThreadID)

		var agentsRes protocol.AgentListResult
		if err := call(protocol.MethodAgentList, protocol.AgentListParams{
			ThreadID:  threadID,
			SessionID: sid,
		}, &agentsRes); err != nil {
			return dataLoadedMsg{err: err}
		}

		var runtimeRes protocol.RuntimeGetSessionStateResult
		if err := call(protocol.MethodRuntimeGetSessionState, protocol.RuntimeGetSessionStateParams{
			SessionID: sid,
		}, &runtimeRes); err != nil {
			return dataLoadedMsg{err: err}
		}

		var totals protocol.SessionGetTotalsResult
		if err := call(protocol.MethodSessionGetTotals, protocol.SessionGetTotalsParams{
			ThreadID: threadID,
			TeamID:   teamID,
			RunID:    runID,
		}, &totals); err != nil {
			return dataLoadedMsg{err: err}
		}

		stats := sessionStats{
			TotalTokens:  totals.TotalTokens,
			TotalCostUSD: totals.TotalCostUSD,
			Assigned:     totals.TasksDone,
			Completed:    totals.TasksDone,
			Done:         totals.TasksDone,
		}

		assignedByRun := map[string]int{}
		completedByRun := map[string]int{}
		assignedByRole := map[string]int{}
		completedByRole := map[string]int{}
		if teamID != "" {
			var manifest protocol.TeamGetManifestResult
			if err := call(protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
				ThreadID: threadID,
				TeamID:   teamID,
			}, &manifest); err == nil {
				reviewerRole = strings.TrimSpace(manifest.ReviewerRole)
			}
			var teamStatus protocol.TeamGetStatusResult
			if err := call(protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
				ThreadID: threadID,
				TeamID:   teamID,
			}, &teamStatus); err != nil {
				return dataLoadedMsg{err: err}
			}

			seenTask := map[string]bool{}
			teamTasks := make([]protocol.Task, 0, 512)
			for _, view := range []string{"inbox", "outbox"} {
				tasks, err := listTasksByView(ctx, scopeClient, protocol.TaskListParams{
					ThreadID: threadID,
					Scope:    "team",
					TeamID:   teamID,
					RunID:    "",
					View:     view,
				})
				if err != nil {
					if rpcscope.IsScopeUnavailable(err) {
						return dataLoadedMsg{preserve: true, connected: true, err: err}
					}
					continue
				}
				for _, t := range tasks {
					taskID := strings.TrimSpace(t.ID)
					if taskID != "" && seenTask[taskID] {
						continue
					}
					if taskID != "" {
						seenTask[taskID] = true
					}
					teamTasks = append(teamTasks, t)
				}
			}
			taskCounts := accumulateTeamTaskCounts(teamTasks)
			assignedByRun = taskCounts.AssignedByRun
			completedByRun = taskCounts.CompletedByRun
			assignedByRole = taskCounts.AssignedByRole
			completedByRole = taskCounts.CompletedByRole
			stats.Pending = taskCounts.Pending
			stats.Active = taskCounts.Active
			stats.Assigned = taskCounts.Assigned
			stats.Completed = taskCounts.Completed
			stats.Done = taskCounts.Done
			if stats.TotalTokens == 0 {
				stats.TotalTokens = teamStatus.TotalTokens
			}
			if stats.TotalCostUSD == 0 {
				stats.TotalCostUSD = teamStatus.TotalCostUSD
			}
		}

		runtimeByRun := make(map[string]protocol.RuntimeRunState, len(runtimeRes.Runs))
		for _, rs := range runtimeRes.Runs {
			rid := strings.TrimSpace(rs.RunID)
			if rid == "" {
				continue
			}
			runtimeByRun[rid] = rs
		}

		agents := make([]agentRow, 0, len(agentsRes.Agents))
		runningFromRows := 0
		for _, agent := range agentsRes.Agents {
			rid := strings.TrimSpace(agent.RunID)
			role := strings.TrimSpace(agent.Role)
			// For subagents, use canonical "Subagent-N" identity
			if strings.TrimSpace(agent.ParentRunID) != "" {
				spawnIndex := agent.SpawnIndex
				if spawnIndex <= 0 {
					spawnIndex = 1
				}
				role = fmt.Sprintf("Subagent-%d", spawnIndex)
			}
			if role == "" {
				role = "-"
			}

			status := strings.TrimSpace(agent.Status)
			worker := false
			model := ""
			runTotalTokens := 0
			runTotalCostUSD := 0.0
			if rs, ok := runtimeByRun[rid]; ok {
				if effective := strings.TrimSpace(rs.EffectiveStatus); effective != "" {
					status = effective
				}
				worker = rs.WorkerPresent
				model = strings.TrimSpace(rs.Model)
				runTotalTokens = rs.RunTotalTokens
				runTotalCostUSD = rs.RunTotalCostUSD
			}

			if isRunningStatus(status) {
				runningFromRows++
			}

			assignedTasks := assignedByRun[rid]
			completedTasks := completedByRun[rid]
			if rid == "" {
				roleKey := strings.ToLower(strings.TrimSpace(role))
				assignedTasks = assignedByRole[roleKey]
				completedTasks = completedByRole[roleKey]
			}

			agents = append(agents, agentRow{
				Role:            kit.Fallback(role, "-"),
				RunID:           rid,
				Status:          kit.Fallback(status, "idle"),
				Profile:         strings.TrimSpace(agent.Profile),
				Model:           kit.Fallback(model, "-"),
				RunTotalTokens:  runTotalTokens,
				RunTotalCostUSD: runTotalCostUSD,
				AssignedTasks:   assignedTasks,
				CompletedTasks:  completedTasks,
				WorkerPresent:   worker,
				StartedAt:       strings.TrimSpace(agent.StartedAt),
			})
		}
		if stats.RunningCount <= 0 {
			stats.RunningCount = runningFromRows
		}
		if stats.Assigned < stats.Completed {
			stats.Assigned = stats.Completed
		}

		return dataLoadedMsg{
			agents:       agents,
			stats:        stats,
			sessionMode:  mode,
			teamID:       teamID,
			runID:        runID,
			reviewerRole: reviewerRole,
			connected:    true,
		}
	}
}

func fetchProjectDataCmd(endpoint, projectRoot string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}
		call := func(method string, params, out any) error {
			return cli.Call(ctx, method, params, out)
		}

		projectRoot = strings.TrimSpace(projectRoot)
		if projectRoot == "" {
			return projectDataLoadedMsg{err: fmt.Errorf("project root is required")}
		}

		// Fetch project context for project ID.
		var projCtx protocol.ProjectGetContextResult
		_ = call(protocol.MethodProjectGetContext, protocol.ProjectGetContextParams{
			Cwd: projectRoot,
		}, &projCtx)

		// List all teams for this project.
		var teamsResult protocol.ProjectListTeamsResult
		if err := call(protocol.MethodProjectListTeams, protocol.ProjectListTeamsParams{
			ProjectRoot: projectRoot,
		}, &teamsResult); err != nil {
			return projectDataLoadedMsg{err: fmt.Errorf("rpc %s: %w", protocol.MethodProjectListTeams, err)}
		}

		// Enrich each team with status, manifest, and runtime state.
		rows := make([]teamRow, 0, len(teamsResult.Teams))
		for _, summary := range teamsResult.Teams {
			row := teamRow{
				TeamID:           strings.TrimSpace(summary.TeamID),
				ProfileID:        strings.TrimSpace(summary.ProfileID),
				PrimarySessionID: strings.TrimSpace(summary.PrimarySessionID),
				CoordinatorRunID: strings.TrimSpace(summary.CoordinatorRunID),
				Status:           strings.TrimSpace(summary.Status),
				ManifestPresent:  summary.ManifestPresent,
				CreatedAt:        strings.TrimSpace(summary.CreatedAt),
				UpdatedAt:        strings.TrimSpace(summary.UpdatedAt),
			}

			sessionID := row.PrimarySessionID
			if sessionID != "" && row.TeamID != "" {
				threadID := protocol.ThreadID(sessionID)

				// Team task counts and cost.
				var teamStatus protocol.TeamGetStatusResult
				if err := call(protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
					ThreadID: threadID,
					TeamID:   row.TeamID,
				}, &teamStatus); err == nil {
					row.Pending = teamStatus.Pending
					row.Active = teamStatus.Active
					row.Done = teamStatus.Done
					row.TotalTokens = teamStatus.TotalTokens
					row.TotalCostUSD = teamStatus.TotalCostUSD
					row.TotalAgents = len(teamStatus.RunIDs)
				}

				// Coordinator role from manifest.
				var manifest protocol.TeamGetManifestResult
				if err := call(protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
					ThreadID: threadID,
					TeamID:   row.TeamID,
				}, &manifest); err == nil {
					row.CoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
				}

				// Running agent counts and coordinator effective status.
				var runtimeRes protocol.RuntimeGetSessionStateResult
				if err := call(protocol.MethodRuntimeGetSessionState, protocol.RuntimeGetSessionStateParams{
					SessionID: sessionID,
				}, &runtimeRes); err == nil {
					running := 0
					for _, rs := range runtimeRes.Runs {
						if isRunningStatus(rs.EffectiveStatus) {
							running++
						}
						if strings.TrimSpace(rs.RunID) == row.CoordinatorRunID {
							row.CoordinatorStatus = strings.TrimSpace(rs.EffectiveStatus)
						}
					}
					row.RunningAgents = running
					if row.TotalAgents == 0 {
						row.TotalAgents = len(runtimeRes.Runs)
					}
				}

				row.HasBlockedTasks = row.Pending > 0 && row.RunningAgents == 0
			}

			rows = append(rows, row)
		}

		return projectDataLoadedMsg{
			teams:     rows,
			projectID: strings.TrimSpace(projCtx.Context.Config.ProjectID),
		}
	}
}

func syncSessionCmd(projectRoot, endpoint, currentSessionID string) tea.Cmd {
	return func() tea.Msg {
		nextID, err := sessionsync.ResolveActiveSessionID(projectRoot, endpoint)
		if err != nil {
			return sessionSyncedMsg{sessionID: strings.TrimSpace(currentSessionID), err: err}
		}
		nextID = strings.TrimSpace(nextID)
		currentSessionID = strings.TrimSpace(currentSessionID)
		return sessionSyncedMsg{
			sessionID: nextID,
			changed:   nextID != "" && nextID != currentSessionID,
		}
	}
}

func taskAssignedRole(t protocol.Task) string {
	role := strings.ToLower(strings.TrimSpace(t.AssignedRole))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(t.RoleSnapshot))
	}
	if role == "" && strings.EqualFold(strings.TrimSpace(t.AssignedToType), "role") {
		role = strings.ToLower(strings.TrimSpace(t.AssignedTo))
	}
	return role
}

func listTasksByView(ctx context.Context, client *rpcscope.Client, base protocol.TaskListParams) ([]protocol.Task, error) {
	const pageSize = 2000
	out := make([]protocol.Task, 0, pageSize)
	offset := 0
	for {
		var page protocol.TaskListResult
		params := base
		params.Limit = pageSize
		params.Offset = offset
		if err := client.Call(ctx, protocol.MethodTaskList, params, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Tasks...)
		if len(page.Tasks) < pageSize {
			break
		}
		offset += len(page.Tasks)
	}
	return out, nil
}

func isCompletedTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "failed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func accumulateTeamTaskCounts(tasks []protocol.Task) dashboardTaskCounts {
	counts := dashboardTaskCounts{
		AssignedByRun:   map[string]int{},
		CompletedByRun:  map[string]int{},
		AssignedByRole:  map[string]int{},
		CompletedByRole: map[string]int{},
	}
	for _, task := range tasks {
		if isStagedCallbackChildTask(task) {
			continue
		}
		assignedRunID := strings.TrimSpace(string(task.RunID))
		if assignedRunID != "" {
			counts.AssignedByRun[assignedRunID]++
			if isCompletedTaskStatus(task.Status) {
				counts.CompletedByRun[assignedRunID]++
			}
		} else {
			role := taskAssignedRole(task)
			if role != "" {
				counts.AssignedByRole[role]++
				if isCompletedTaskStatus(task.Status) {
					counts.CompletedByRole[role]++
				}
			}
		}

		status := strings.ToLower(strings.TrimSpace(task.Status))
		switch status {
		case "pending":
			counts.Pending++
		case "active", "review_pending":
			counts.Active++
		default:
			if isCompletedTaskStatus(status) {
				counts.Done++
			}
		}
	}
	counts.Assigned = counts.Pending + counts.Active + counts.Done
	counts.Completed = counts.Done
	return counts
}

func isStagedCallbackChildTask(task protocol.Task) bool {
	return task.BatchMode && !task.BatchSynthetic && strings.TrimSpace(task.BatchIncludedIn) != ""
}

func tickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func isRunningStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "active", "thinking", "working":
		return true
	default:
		return false
	}
}
