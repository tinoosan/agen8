package coordinator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

const detachedThreadID = protocol.ThreadID("detached-control")

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
	goal string
	err  error
}

type sessionActionMsg struct {
	action string
	err    error
}

type tickMsg struct{}

func fetchSessionCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		call := func(method string, params, out any) error {
			if err := cli.Call(ctx, method, params, out); err != nil {
				return fmt.Errorf("rpc %s: %w", method, err)
			}
			return nil
		}

		sid := strings.TrimSpace(sessionID)
		if sid == "" {
			return sessionLoadedMsg{err: fmt.Errorf("session id is required")}
		}

		var out protocol.SessionListResult
		if err := call(protocol.MethodSessionList, protocol.SessionListParams{
			ThreadID: detachedThreadID,
			Limit:    500,
			Offset:   0,
		}, &out); err != nil {
			return sessionLoadedMsg{err: err}
		}

		var item *protocol.SessionListItem
		for i := range out.Sessions {
			if strings.TrimSpace(out.Sessions[i].SessionID) == sid {
				item = &out.Sessions[i]
				break
			}
		}
		if item == nil {
			return sessionLoadedMsg{err: fmt.Errorf("session %q not found", sid)}
		}

		msg := sessionLoadedMsg{
			sessionMode: fallback(strings.TrimSpace(item.Mode), "standalone"),
			teamID:      strings.TrimSpace(item.TeamID),
			runID:       strings.TrimSpace(item.CurrentRunID),
			connected:   true,
		}

		if msg.teamID != "" {
			var manifest protocol.TeamGetManifestResult
			if err := call(protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
				ThreadID: protocol.ThreadID(sid),
				TeamID:   msg.teamID,
			}, &manifest); err == nil {
				if strings.TrimSpace(manifest.CoordinatorRun) != "" {
					msg.runID = strings.TrimSpace(manifest.CoordinatorRun)
				}
				msg.coordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
			}
		}

		return msg
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
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		goal = strings.TrimSpace(goal)
		if goal == "" {
			return goalSubmittedMsg{goal: goal, err: fmt.Errorf("goal is required")}
		}

		var out protocol.TaskCreateResult
		err := callWithThreadRecovery(ctx, cli, sessionID, runID, func(threadID protocol.ThreadID) error {
			return cli.Call(ctx, protocol.MethodTaskCreate, protocol.TaskCreateParams{
				ThreadID:     threadID,
				TeamID:       strings.TrimSpace(teamID),
				RunID:        strings.TrimSpace(runID),
				Goal:         goal,
				TaskKind:     "user_message",
				AssignedRole: strings.TrimSpace(coordinatorRole),
			}, &out)
		})
		if err != nil {
			return goalSubmittedMsg{goal: goal, err: fmt.Errorf("rpc task.create: %w", err)}
		}
		return goalSubmittedMsg{goal: goal}
	}
}

func sessionActionCmd(endpoint, sessionID, runID string, action string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		sid := strings.TrimSpace(sessionID)
		action = strings.ToLower(strings.TrimSpace(action))

		switch action {
		case "pause":
			var out protocol.SessionPauseResult
			err := callWithThreadRecovery(ctx, cli, sessionID, runID, func(threadID protocol.ThreadID) error {
				return cli.Call(ctx, protocol.MethodSessionPause, protocol.SessionPauseParams{ThreadID: threadID, SessionID: sid}, &out)
			})
			if err != nil {
				return sessionActionMsg{action: action, err: fmt.Errorf("rpc session.pause: %w", err)}
			}
		case "resume":
			var out protocol.SessionResumeResult
			err := callWithThreadRecovery(ctx, cli, sessionID, runID, func(threadID protocol.ThreadID) error {
				return cli.Call(ctx, protocol.MethodSessionResume, protocol.SessionResumeParams{ThreadID: threadID, SessionID: sid}, &out)
			})
			if err != nil {
				return sessionActionMsg{action: action, err: fmt.Errorf("rpc session.resume: %w", err)}
			}
		case "stop":
			var out protocol.SessionStopResult
			err := callWithThreadRecovery(ctx, cli, sessionID, runID, func(threadID protocol.ThreadID) error {
				return cli.Call(ctx, protocol.MethodSessionStop, protocol.SessionStopParams{ThreadID: threadID, SessionID: sid}, &out)
			})
			if err != nil {
				return sessionActionMsg{action: action, err: fmt.Errorf("rpc session.stop: %w", err)}
			}
		default:
			return sessionActionMsg{action: action, err: fmt.Errorf("unknown action %q", action)}
		}

		return sessionActionMsg{action: action}
	}
}

func callWithThreadRecovery(ctx context.Context, cli protocol.TCPClient, sessionID, runID string, fn func(threadID protocol.ThreadID) error) error {
	sid := strings.TrimSpace(sessionID)
	threadID := protocol.ThreadID(sid)
	err := fn(threadID)
	if err == nil || !isThreadNotFound(err) {
		return err
	}

	var resolved protocol.SessionResolveThreadResult
	if rerr := cli.Call(ctx, protocol.MethodSessionResolveThread, protocol.SessionResolveThreadParams{
		SessionID: sid,
		RunID:     strings.TrimSpace(runID),
	}, &resolved); rerr != nil {
		return err
	}
	if strings.TrimSpace(resolved.ThreadID) == "" {
		return err
	}
	return fn(protocol.ThreadID(strings.TrimSpace(resolved.ThreadID)))
}

func isThreadNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "thread not found")
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
