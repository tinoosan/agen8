package dashboardtui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
)

const detachedThreadID = protocol.ThreadID("detached-control")

type agentRow struct {
	Role            string
	RunID           string
	Status          string
	Profile         string
	AssignedTasks   int
	CompletedTasks  int
	WorkerPresent   bool
	LastHeartbeatAt string
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
	agents      []agentRow
	stats       sessionStats
	sessionMode string
	teamID      string
	runID       string
	connected   bool
	err         error
}

type tickMsg struct{}

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

		session, err := fetchSessionItem(call, sid)
		if err != nil {
			return dataLoadedMsg{err: err}
		}

		mode := fallback(strings.TrimSpace(session.Mode), "standalone")
		teamID := strings.TrimSpace(session.TeamID)
		runID := strings.TrimSpace(session.CurrentRunID)
		threadID := protocol.ThreadID(sid)

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

		assignedByRole := map[string]int{}
		completedByRole := map[string]int{}
		if teamID != "" {
			var teamStatus protocol.TeamGetStatusResult
			if err := call(protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
				ThreadID: threadID,
				TeamID:   teamID,
			}, &teamStatus); err != nil {
				return dataLoadedMsg{err: err}
			}

			if teamID != "" {
				seenTask := map[string]bool{}
				views := []string{"inbox", "outbox"}
				for _, view := range views {
					var taskRes protocol.TaskListResult
					if err := call(protocol.MethodTaskList, protocol.TaskListParams{
						ThreadID: threadID,
						TeamID:   teamID,
						View:     view,
						Limit:    1000,
						Offset:   0,
					}, &taskRes); err != nil {
						continue
					}
					for _, t := range taskRes.Tasks {
						taskID := strings.TrimSpace(t.ID)
						if taskID != "" && seenTask[taskID] {
							continue
						}
						if taskID != "" {
							seenTask[taskID] = true
						}
						role := strings.ToLower(strings.TrimSpace(t.AssignedRole))
						if role == "" {
							role = strings.ToLower(strings.TrimSpace(t.RoleSnapshot))
						}
						if role == "" && strings.EqualFold(strings.TrimSpace(t.AssignedToType), "role") {
							role = strings.ToLower(strings.TrimSpace(t.AssignedTo))
						}
						if role == "" {
							continue
						}
						assignedByRole[role]++
						if isCompletedTaskStatus(t.Status) {
							completedByRole[role]++
						}
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
			if strings.EqualFold(mode, "standalone") && role == "" {
				role = "-"
			}

			status := strings.TrimSpace(agent.Status)
			worker := false
			heartbeat := ""
			if rs, ok := runtimeByRun[rid]; ok {
				if effective := strings.TrimSpace(rs.EffectiveStatus); effective != "" {
					status = effective
				}
				worker = rs.WorkerPresent
				heartbeat = strings.TrimSpace(rs.LastHeartbeatAt)
			}

			if isRunningStatus(status) {
				runningFromRows++
			}

			agents = append(agents, agentRow{
				Role:            fallback(role, "-"),
				RunID:           rid,
				Status:          fallback(status, "idle"),
				Profile:         strings.TrimSpace(agent.Profile),
				AssignedTasks:   assignedByRole[strings.ToLower(strings.TrimSpace(role))],
				CompletedTasks:  completedByRole[strings.ToLower(strings.TrimSpace(role))],
				WorkerPresent:   worker,
				LastHeartbeatAt: heartbeat,
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
			agents:      agents,
			stats:       stats,
			sessionMode: mode,
			teamID:      teamID,
			runID:       runID,
			connected:   true,
		}
	}
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
