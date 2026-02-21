package coordinator

import (
	"context"
	"fmt"
	"regexp"
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
	feedThinking
)

type feedEntry struct {
	kind           feedKind
	timestamp      time.Time
	finishedAt     time.Time // zero if not yet finished
	role           string
	text           string
	path           string
	status         string
	opKind         string
	sourceID       string
	isText         bool
	isTaskResponse bool              // true if this text response came from the top-level task (prevents activity merge dupe)
	data           map[string]string // raw activity Data for verb resolution
	planItems      []string          // parsed checklist items for plan writes
	childCount     int               // number of grouped bridge tool calls (for code_exec parents)
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

type userTasksLoadedMsg struct {
	entries []feedEntry
	err     error
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

type thinkingEventsMsg struct {
	entries []feedEntry
	lastSeq int64
	err     error
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
			// Suppress tool operation writes to SUMMARY.md since we natively display the task.done response block.
			if isActivityPlanWrite(act.Kind, act.Path) || isActivitySummaryWrite(act.Kind, act.Path) {
				if isActivitySummaryWrite(act.Kind, act.Path) {
					continue
				}
			}

			ts := act.StartedAt
			if ts.IsZero() {
				ts = time.Now()
			}
			var fin time.Time
			if act.FinishedAt != nil {
				fin = *act.FinishedAt
			}
			var kind feedKind = feedAgent
			if strings.TrimSpace(strings.ToLower(act.Kind)) == "user_message" {
				kind = feedUser
			}

			entries = append(entries, feedEntry{
				kind:       kind,
				timestamp:  ts,
				finishedAt: fin,
				role:       activityRole(act),
				text:       activityText(act),
				path:       strings.TrimSpace(act.Path),
				status:     string(act.Status),
				opKind:     strings.TrimSpace(act.Kind),
				sourceID:   strings.TrimSpace(act.ID),
				isText:     isActivityText(act),
				data:       act.Data,
			})
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].timestamp.Before(entries[j].timestamp)
		})

		// If any activity is a plan write, fetch the current plan checklist.
		hasPlanWrite := false
		lastPlanIdx := -1
		for i, e := range entries {
			if isActivityPlanWrite(e.opKind, e.text) {
				hasPlanWrite = true
				lastPlanIdx = i
			}
		}
		if hasPlanWrite && lastPlanIdx >= 0 {
			var planRes protocol.PlanGetResult
			if err := cli.Call(ctx, protocol.MethodPlanGet, protocol.PlanGetParams{
				ThreadID: protocol.ThreadID(sid),
			}, &planRes); err == nil && planRes.Checklist != "" {
				entries[lastPlanIdx].planItems = parseChecklistItems(planRes.Checklist)
			}
		}

		return activityLoadedMsg{entries: entries, connected: true}
	}
}

func fetchUserTasksCmd(endpoint, threadID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		tid := strings.TrimSpace(threadID)
		if tid == "" {
			return userTasksLoadedMsg{err: fmt.Errorf("thread id is required")}
		}

		var res protocol.TaskListResult
		if err := cli.Call(ctx, protocol.MethodTaskList, protocol.TaskListParams{
			ThreadID: protocol.ThreadID(tid),
			Limit:    100, // typically few user messages per session
		}, &res); err != nil {
			return userTasksLoadedMsg{err: fmt.Errorf("rpc task.list: %w", err)}
		}

		var entries []feedEntry
		for _, task := range res.Tasks {
			if strings.TrimSpace(strings.ToLower(task.TaskKind)) != "user_message" {
				continue
			}

			ts := task.CreatedAt
			if ts.IsZero() {
				ts = time.Now()
			}
			var fin time.Time
			if !task.CompletedAt.IsZero() {
				fin = task.CompletedAt
			}

			text := strings.TrimSpace(task.Goal)

			entries = append(entries, feedEntry{
				kind:       feedUser,
				timestamp:  ts,
				finishedAt: fin,
				role:       "You",
				text:       text,
				status:     string(task.Status),
				opKind:     task.TaskKind,
				sourceID:   string(task.ID),
				isText:     true,
			})
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].timestamp.Before(entries[j].timestamp)
		})

		return userTasksLoadedMsg{entries: entries}
	}
}

func fetchThinkingEventsCmd(endpoint, runID string, afterSeq int64) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(runID) == "" {
			return thinkingEventsMsg{}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}

		var res protocol.EventsListPaginatedResult
		if err := cli.Call(ctx, protocol.MethodEventsListPaginated, protocol.EventsListPaginatedParams{
			RunID:    strings.TrimSpace(runID),
			AfterSeq: afterSeq,
			Limit:    100,
			Types:    []string{"model.thinking.start", "model.thinking.end", "task.done"},
		}, &res); err != nil {
			return thinkingEventsMsg{err: err}
		}

		var entries []feedEntry
		var maxSeq int64
		for _, ev := range res.Events {
			if ev.Type == "model.thinking.start" {
				entries = append(entries, feedEntry{
					kind:      feedThinking,
					timestamp: ev.Timestamp,
					text:      "Thinking",
					sourceID:  ev.EventID,
				})
			} else if ev.Type == "task.done" {
				summary := strings.TrimSpace(ev.Data["summary"])
				if summary == "" {
					summary = "(Task completed.)"
				}
				taskId := strings.TrimSpace(ev.Data["taskId"])

				role := strings.TrimSpace(ev.Data["agent"])
				if role == "" {
					role = strings.TrimSpace(ev.Data["role"])
				}
				if role == "" {
					role = strings.TrimSpace(ev.Data["agent_role"])
				}
				if role == "" {
					role = "agent"
				}

				entries = append(entries, feedEntry{
					kind:           feedAgent,
					isText:         true,
					isTaskResponse: true,
					text:           summary,
					role:           role,
					timestamp:      ev.Timestamp,
					sourceID:       taskId,
				})
			}
			seq := res.Next
			if seq > maxSeq {
				maxSeq = seq
			}
		}

		return thinkingEventsMsg{entries: entries, lastSeq: maxSeq}
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
	path := strings.TrimSpace(act.Path)
	if path != "" {
		return path
	}
	return ""
}

func isActivityText(act types.Activity) bool {
	kind := strings.TrimSpace(act.Kind)
	// If it's explicitly a message kind, it's text.
	if strings.HasSuffix(kind, "_message") {
		return true
	}
	// If it lacks a specific developer op prefix but has a title, treat as text summary.
	if strings.TrimSpace(act.Title) != "" && !strings.HasPrefix(kind, "fs_") && !strings.HasPrefix(kind, "shell_") && kind != "agent_spawn" && kind != "tool_call" && kind != "code_exec" {
		return true
	}
	return false
}

// kindToVerb maps activity kinds to human-friendly verbs.
func kindToVerb(kind string, data map[string]string) string {
	k := strings.TrimSpace(strings.ToLower(kind))
	switch k {
	case "fs_read":
		return "Read"
	case "fs_list":
		return "List"
	case "fs_write":
		return "Write"
	case "fs_append":
		return "Append"
	case "fs_edit":
		return "Edit"
	case "fs_patch":
		return "Patch"
	case "fs_search":
		return "Search"
	case "shell_exec":
		return "Bash"
	case "http_fetch":
		return "Fetch"
	case "browser":
		return "Browse"
	case "code_exec":
		return "Python"
	case "email":
		return "Email"
	case "agent_spawn":
		return "Spawn"
	case "task_create":
		return "Dispatch task"
	case "task_review":
		return "Review task"
	case "trace_run":
		return "Trace"
	case "workdir.changed":
		return "Workdir"
	case "llm.web.search":
		return "Web search"
	}
	// Fallback: use Data["tool"] if available, otherwise raw kind.
	if data != nil {
		if tool := strings.TrimSpace(data["tool"]); tool != "" {
			return tool
		}
	}
	if k != "" {
		return kind
	}
	return "op"
}

// isActivityPlanWrite returns true if the activity represents a plan file write.
func isActivityPlanWrite(kind string, text string) bool {
	k := strings.TrimSpace(strings.ToLower(kind))
	if k != "fs_write" && k != "fs_edit" && k != "fs_patch" && k != "fs_append" {
		return false
	}
	p := strings.TrimSpace(text)
	if p == "" {
		return false
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.EqualFold(p, "/plan/HEAD.md") || strings.EqualFold(p, "/plan/CHECKLIST.md")
}

// isActivitySummaryWrite returns true if the activity represents a task summary write.
func isActivitySummaryWrite(kind string, text string) bool {
	k := strings.TrimSpace(strings.ToLower(kind))
	if k != "fs_write" && k != "fs_edit" && k != "fs_patch" && k != "fs_append" {
		return false
	}
	p := strings.TrimSpace(text)
	return strings.HasSuffix(strings.ToLower(p), "summary.md")
}

// checklistRe matches markdown checklist lines like "- [x] item" or "* [ ] item".
var checklistRe = regexp.MustCompile(`^[\s]*[-*]\s*\[([ xX])\]\s*(.+)$`)

// parseChecklistItems extracts checklist items from markdown content.
func parseChecklistItems(md string) []string {
	var items []string
	for _, line := range strings.Split(md, "\n") {
		if m := checklistRe.FindStringSubmatch(line); m != nil {
			check := strings.ToLower(m[1])
			text := strings.TrimSpace(m[2])
			if check == "x" {
				items = append(items, "[x] "+text)
			} else {
				items = append(items, "[ ] "+text)
			}
		}
	}
	return items
}
