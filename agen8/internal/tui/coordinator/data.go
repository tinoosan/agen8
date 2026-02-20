package coordinator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

type feedKind int

const (
	feedUser feedKind = iota
	feedAgent
	feedSystem
)

type feedEntry struct {
	kind      feedKind
	timestamp time.Time
	role      string
	text      string
	status    string
	opKind    string
	sourceID  string
}

type sessionLoadedMsg struct {
	sessionMode     string
	teamID          string
	runID           string
	threadID        string
	coordinatorRole string
	connected       bool
	err             error
}

type activityLoadedMsg struct {
	entries   []feedEntry
	connected bool
	err       error
}

type goalSubmittedMsg struct {
	goal      string
	scope     rpcscope.ScopeState
	recovered bool
	err       error
}

type sessionActionMsg struct {
	action    string
	scope     rpcscope.ScopeState
	recovered bool
	err       error
}

type tickMsg struct{}

func fetchSessionCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sid := strings.TrimSpace(sessionID)
		if sid == "" {
			return sessionLoadedMsg{err: fmt.Errorf("session id is required")}
		}

		client := rpcscope.NewClient(endpoint, sid).WithTimeout(5 * time.Second)
		scope, err := client.RefreshScope(ctx)
		if err != nil {
			return sessionLoadedMsg{err: err}
		}

		return sessionLoadedMsg{
			sessionMode:     scope.Mode,
			teamID:          scope.TeamID,
			runID:           scope.RunID,
			threadID:        scope.ThreadID,
			coordinatorRole: scope.CoordinatorRole,
			connected:       true,
		}
	}
}

func fetchActivityCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		sid := strings.TrimSpace(sessionID)
		if sid == "" {
			return activityLoadedMsg{err: fmt.Errorf("session id is required")}
		}

		var res protocol.ActivityListResult
		if err := cli.Call(ctx, protocol.MethodActivityList, protocol.ActivityListParams{
			ThreadID:         protocol.ThreadID(sid),
			Limit:            500,
			Offset:           0,
			SortDesc:         false,
			IncludeChildRuns: true,
		}, &res); err != nil {
			return activityLoadedMsg{err: fmt.Errorf("rpc activity.list: %w", err)}
		}

		entries := make([]feedEntry, 0, len(res.Activities))
		for _, act := range res.Activities {
			ts := act.StartedAt
			if ts.IsZero() {
				ts = time.Now()
			}
			entries = append(entries, feedEntry{
				kind:      feedAgent,
				timestamp: ts,
				role:      activityRole(act),
				text:      activityText(act),
				status:    string(act.Status),
				opKind:    strings.TrimSpace(act.Kind),
				sourceID:  strings.TrimSpace(act.ID),
			})
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].timestamp.Before(entries[j].timestamp)
		})

		return activityLoadedMsg{entries: entries, connected: true}
	}
}

func submitGoalCmd(endpoint, sessionID, teamID, runID, coordinatorRole, goal string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		goal = strings.TrimSpace(goal)
		if goal == "" {
			return goalSubmittedMsg{goal: goal, err: fmt.Errorf("goal is required")}
		}

		resolvedSessionID := strings.TrimSpace(sessionID)
		if strings.TrimSpace(teamID) != "" {
			sid, err := rpcscope.ResolveControlSessionID(ctx, endpoint, resolvedSessionID, strings.TrimSpace(teamID))
			if err == nil {
				resolvedSessionID = sid
			}
		}

		client := rpcscope.NewClient(endpoint, resolvedSessionID).WithTimeout(5 * time.Second)
		client.SetState(rpcscope.ScopeState{
			SessionID:       strings.TrimSpace(resolvedSessionID),
			ThreadID:        strings.TrimSpace(resolvedSessionID),
			TeamID:          strings.TrimSpace(teamID),
			RunID:           strings.TrimSpace(runID),
			CoordinatorRole: strings.TrimSpace(coordinatorRole),
		})

		var out protocol.TaskCreateResult
		scope, recovered, err := client.CallWithRecovery(ctx, protocol.MethodTaskCreate, func(scope rpcscope.ScopeState) (any, error) {
			effectiveRole := strings.TrimSpace(scope.CoordinatorRole)
			if effectiveRole == "" {
				effectiveRole = strings.TrimSpace(coordinatorRole)
			}
			return protocol.TaskCreateParams{
				ThreadID:     protocol.ThreadID(scope.ThreadID),
				TeamID:       strings.TrimSpace(scope.TeamID),
				RunID:        strings.TrimSpace(scope.RunID),
				Goal:         goal,
				TaskKind:     "user_message",
				AssignedRole: effectiveRole,
			}, nil
		}, &out)
		if err != nil {
			return goalSubmittedMsg{goal: goal, scope: scope, recovered: recovered, err: err}
		}
		return goalSubmittedMsg{goal: goal, scope: scope, recovered: recovered}
	}
}

func sessionActionCmd(endpoint, sessionID, teamID string, action string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sid := strings.TrimSpace(sessionID)
		action = strings.ToLower(strings.TrimSpace(action))
		if strings.TrimSpace(teamID) != "" {
			resolved, err := rpcscope.ResolveControlSessionID(ctx, endpoint, sid, strings.TrimSpace(teamID))
			if err != nil {
				return sessionActionMsg{action: action, err: fmt.Errorf("team control session unavailable; refresh manifest or reconnect")}
			}
			sid = strings.TrimSpace(resolved)
		}
		if sid == "" {
			return sessionActionMsg{action: action, err: fmt.Errorf("session id is required")}
		}
		client := rpcscope.NewClient(endpoint, sid).WithTimeout(5 * time.Second)

		switch action {
		case "pause":
			var out protocol.SessionPauseResult
			scope, recovered, err := client.CallWithRecovery(ctx, protocol.MethodSessionPause, func(scope rpcscope.ScopeState) (any, error) {
				return protocol.SessionPauseParams{ThreadID: protocol.ThreadID(scope.ThreadID), SessionID: scope.SessionID}, nil
			}, &out)
			if err != nil {
				return sessionActionMsg{action: action, scope: scope, recovered: recovered, err: err}
			}
			return sessionActionMsg{action: action, scope: scope, recovered: recovered}
		case "resume":
			var out protocol.SessionResumeResult
			scope, recovered, err := client.CallWithRecovery(ctx, protocol.MethodSessionResume, func(scope rpcscope.ScopeState) (any, error) {
				return protocol.SessionResumeParams{ThreadID: protocol.ThreadID(scope.ThreadID), SessionID: scope.SessionID}, nil
			}, &out)
			if err != nil {
				return sessionActionMsg{action: action, scope: scope, recovered: recovered, err: err}
			}
			return sessionActionMsg{action: action, scope: scope, recovered: recovered}
		case "stop":
			var out protocol.SessionStopResult
			scope, recovered, err := client.CallWithRecovery(ctx, protocol.MethodSessionStop, func(scope rpcscope.ScopeState) (any, error) {
				return protocol.SessionStopParams{ThreadID: protocol.ThreadID(scope.ThreadID), SessionID: scope.SessionID}, nil
			}, &out)
			if err != nil {
				return sessionActionMsg{action: action, scope: scope, recovered: recovered, err: err}
			}
			return sessionActionMsg{action: action, scope: scope, recovered: recovered}
		default:
			return sessionActionMsg{action: action, err: fmt.Errorf("unknown action %q", action)}
		}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func activityRole(act types.Activity) string {
	role := strings.TrimSpace(act.Data["role"])
	if role == "" {
		role = strings.TrimSpace(act.Data["agent_role"])
	}
	if role == "" {
		return "agent"
	}
	return role
}

func activityText(act types.Activity) string {
	title := strings.TrimSpace(act.Title)
	if title != "" {
		return title
	}
	kind := strings.TrimSpace(act.Kind)
	if kind == "" {
		kind = "op"
	}
	path := strings.TrimSpace(act.Path)
	if path != "" {
		return kind + " " + path
	}
	return kind
}
