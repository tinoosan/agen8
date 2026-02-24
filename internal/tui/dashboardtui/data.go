package dashboardtui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	"github.com/tinoosan/agen8/internal/tui/sessionsync"
	"github.com/tinoosan/agen8/pkg/protocol"
)

const detachedThreadID = protocol.ThreadID("detached-control")

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

type tickMsg struct{}

type sessionSyncedMsg struct {
	sessionID string
	changed   bool
	err       error
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
		mode := fallback(strings.TrimSpace(scope.Mode), "standalone")
		teamID := strings.TrimSpace(scope.TeamID)
		runID := strings.TrimSpace(scope.RunID)
		reviewerRole := ""
		threadID := protocol.ThreadID(scope.ThreadID)

		session, err := fetchSessionItem(call, sid)
		if err != nil {
			return dataLoadedMsg{err: err}
		}

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
			RunningCount: session.RunningAgents,
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
					assignedRunID := strings.TrimSpace(string(t.RunID))
					if assignedRunID != "" {
						assignedByRun[assignedRunID]++
						if isCompletedTaskStatus(t.Status) {
							completedByRun[assignedRunID]++
						}
						continue
					}
					role := taskAssignedRole(t)
					if role == "" {
						continue
					}
					assignedByRole[role]++
					if isCompletedTaskStatus(t.Status) {
						completedByRole[role]++
					}
				}
			}
			stats.Pending = teamStatus.Pending
			stats.Active = teamStatus.Active
			stats.Assigned = teamStatus.Pending + teamStatus.Active + teamStatus.Done
			stats.Completed = teamStatus.Done
			stats.Done = teamStatus.Done
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
			if strings.EqualFold(mode, "standalone") && role == "" {
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
				Role:            fallback(role, "-"),
				RunID:           rid,
				Status:          fallback(status, "idle"),
				Profile:         strings.TrimSpace(agent.Profile),
				Model:           fallback(model, "-"),
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

func syncSessionCmd(projectRoot, currentSessionID string) tea.Cmd {
	return func() tea.Msg {
		nextID, err := sessionsync.ResolveActiveSessionID(projectRoot)
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

func fetchSessionItem(call func(method string, params, out any) error, sessionID string) (protocol.SessionListItem, error) {
	var out protocol.SessionListResult
	if err := call(protocol.MethodSessionList, protocol.SessionListParams{
		ThreadID: detachedThreadID,
		Limit:    500,
		Offset:   0,
	}, &out); err != nil {
		return protocol.SessionListItem{}, err
	}
	for i := range out.Sessions {
		if strings.TrimSpace(out.Sessions[i].SessionID) == sessionID {
			return out.Sessions[i], nil
		}
	}
	return protocol.SessionListItem{}, fmt.Errorf("session %q not found", sessionID)
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
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
