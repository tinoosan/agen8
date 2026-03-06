package activitytui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/sessionsync"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

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

type dataLoadedMsg struct {
	activities []types.Activity
	totalCount int
	connected  bool
	preserve   bool
	err        error
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

func fetchDataCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cli := protocol.TCPClient{
			Endpoint: endpoint,
			Timeout:  5 * time.Second,
		}

		threadID := protocol.ThreadID(strings.TrimSpace(sessionID))

		var res protocol.ActivityListResult
		if err := cli.Call(ctx, protocol.MethodActivityList, protocol.ActivityListParams{
			ThreadID:         threadID,
			Limit:            500,
			Offset:           0,
			SortDesc:         false,
			IncludeChildRuns: true,
		}, &res); err != nil {
			return dataLoadedMsg{err: fmt.Errorf("rpc activity.list: %w", err)}
		}

		return dataLoadedMsg{
			activities: res.Activities,
			totalCount: res.TotalCount,
			connected:  true,
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

func isRunningStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "active", "thinking", "working":
		return true
	default:
		return false
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
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
