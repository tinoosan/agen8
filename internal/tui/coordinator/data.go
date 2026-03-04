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
	identityKey    string
	isText         bool
	isTaskResponse bool              // true if this text response came from the top-level task (prevents activity merge dupe)
	data           map[string]string // raw activity Data for verb resolution
	planItems      []string          // parsed checklist items for plan writes
	childCount     int               // number of grouped bridge tool calls (for code_exec parents)
	// When childCount == 1, store the single bridge call so we can show "Verb + Args" instead of "Ran 1 tools".
	bridgeSingleOpKind string
	bridgeSingleData   map[string]string
	bridgeSingleText   string
	bridgeSinglePath   string
	// All bridge children that are write ops, collected regardless of childCount, for diff display.
	bridgeWriteEntries []feedEntry

	// Thinking-specific fields
	live             bool          // true while model is still thinking (no .end yet)
	thinkingDuration time.Duration // elapsed duration once .end arrives
	thinkingLines    []string      // accumulated summary lines from model.thinking.summary
	thinkingStep     string        // agent loop step number (for cross-batch event correlation)
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

type reconnectNotificationMsg struct{}

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
	events  []types.EventRecord
	entries []feedEntry // task.done entries processed in fetch
	lastSeq int64
	err     error
}

type modelSetMsg struct {
	model    string
	accepted bool
	applied  int
	err      error
}

type tickMsg struct{}
type animTickMsg struct{}

func setModelCmd(endpoint, sessionID, model string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		model = strings.TrimSpace(model)
		if model == "" {
			return modelSetMsg{err: fmt.Errorf("model is required")}
		}
		sid := strings.TrimSpace(sessionID)
		if sid == "" {
			return modelSetMsg{err: fmt.Errorf("session id is required")}
		}

		cli := protocol.TCPClient{Endpoint: endpoint, Timeout: 5 * time.Second}
		var out protocol.ControlSetModelResult
		if err := cli.Call(ctx, protocol.MethodControlSetModel, protocol.ControlSetModelParams{
			ThreadID: protocol.ThreadID(sid),
			Model:    model,
		}, &out); err != nil {
			return modelSetMsg{model: model, err: err}
		}
		return modelSetMsg{
			model:    model,
			accepted: out.Accepted,
			applied:  len(out.AppliedTo),
		}
	}
}

func activityToFeedEntry(act types.Activity) *feedEntry {
	if isActivityPlanWrite(act.Kind, act.Path) || isActivitySummaryWrite(act.Kind, act.Path) {
		if isActivitySummaryWrite(act.Kind, act.Path) {
			return nil
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
	kindName := strings.TrimSpace(strings.ToLower(act.Kind))
	if kindName == "user_message" {
		kind = feedUser
	}
	text := activityText(act)
	isTaskResponse := kindName == "task.done"
	if isTaskResponse && isTaskDonePlaceholder(text) {
		return nil
	}
	sourceID := strings.TrimSpace(act.ID)
	if isTaskResponse {
		if taskID := strings.TrimSpace(act.Data["taskId"]); taskID != "" {
			sourceID = taskID
		}
	}

	entry := &feedEntry{
		kind:           kind,
		timestamp:      ts,
		finishedAt:     fin,
		role:           activityRole(act),
		text:           text,
		path:           strings.TrimSpace(act.Path),
		status:         string(act.Status),
		opKind:         strings.TrimSpace(act.Kind),
		sourceID:       sourceID,
		isText:         isActivityText(act),
		isTaskResponse: isTaskResponse,
		data:           act.Data,
	}
	return normalizeFeedEntry(entry)
}

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
			if entry := activityToFeedEntry(act); entry != nil {
				entries = append(entries, *entry)
			}
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].timestamp.Before(entries[j].timestamp)
		})

		// If any activity is a plan write, fetch the current plan checklist.
		hasPlanWrite := false
		lastPlanIdx := -1
		for i, e := range entries {
			if isActivityPlanWrite(e.opKind, e.path) {
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
			Limit:    500,
			Types:    []string{"model.thinking.start", "model.thinking.end", "model.thinking.summary", "task.done"},
		}, &res); err != nil {
			return thinkingEventsMsg{err: err}
		}

		var thinkingEvents []types.EventRecord
		var entries []feedEntry
		var maxSeq int64
		for _, ev := range res.Events {
			switch ev.Type {
			case "model.thinking.start", "model.thinking.summary", "model.thinking.end":
				thinkingEvents = append(thinkingEvents, ev)
			case "task.done":
				summary := strings.TrimSpace(ev.Data["summary"])
				if summary == "" {
					continue
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

		return thinkingEventsMsg{events: thinkingEvents, entries: entries, lastSeq: maxSeq}
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

func animTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func activityRole(act types.Activity) string {
	if strings.TrimSpace(strings.ToLower(act.Kind)) == "user_message" {
		return "You"
	}
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
	if strings.TrimSpace(strings.ToLower(act.Kind)) == "user_message" {
		if v := strings.TrimSpace(act.TextPreview); v != "" {
			return v
		}
	}
	if act.Kind == "task.done" || act.Kind == "agent_speak" {
		if v := strings.TrimSpace(act.OutputPreview); v != "" {
			return v
		}
	}
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
	kind := strings.TrimSpace(strings.ToLower(act.Kind))
	if kind == "user_message" || kind == "task.done" || kind == "task.create" || kind == "agent_speak" || kind == "model_response" {
		return true
	}
	// If it's explicitly a message kind, it's text.
	if strings.HasSuffix(kind, "_message") {
		return true
	}
	// If it lacks a specific developer op prefix but has a title, treat as text summary.
	if strings.TrimSpace(act.Title) != "" && !strings.HasPrefix(kind, "fs_") && !strings.HasPrefix(kind, "shell_") && kind != "agent_spawn" && kind != "tool_call" && kind != "tool_result" && kind != "code_exec" && kind != "http_fetch" && kind != "task_create" && kind != "task_review" && kind != "obsidian" && kind != "soul_update" {
		return true
	}
	return false
}

func isTaskDonePlaceholder(text string) bool {
	trimmed := strings.TrimSpace(text)
	switch trimmed {
	case "", "Task finished", "(Task completed.)":
		return true
	default:
		return false
	}
}

// normPath returns a path with a leading slash for consistent prefix checks.
func normPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// skillNameFromPath returns the first path segment after /skills/ for display (e.g. "Learning <skill>").
// Example: /skills/notion-meeting-intelligence/SKILL.md -> "notion-meeting-intelligence".
func skillNameFromPath(path string) string {
	p := normPath(path)
	if p == "" || !strings.HasPrefix(p, "/skills/") {
		return ""
	}
	rest := strings.TrimPrefix(p, "/skills/")
	if rest == "" {
		return ""
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}

// kindToVerb maps activity kinds to human-friendly verbs. Path is used for /memory and /skills display overrides.
func kindToVerb(kind string, path string, data map[string]string) string {
	k := strings.TrimSpace(strings.ToLower(kind))
	p := normPath(path)
	// /memory: Append or Write -> "Updated memory"; Read or Search -> "Remembering"
	if strings.HasPrefix(p, "/memory/") {
		if k == "fs_write" || k == "fs_append" {
			return "Updated memory"
		}
		if k == "fs_read" || k == "fs_search" {
			return "Remembering"
		}
	}
	// /skills: Read -> "Learning <skill>" (verb is "Learning", arg is skill name)
	if strings.HasPrefix(p, "/skills/") && k == "fs_read" {
		return "Learning"
	}
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
		return "Fetched"
	case "browser":
		return "Browse"
	case "code_exec":
		return "Python"
	case "email":
		return "Email"
	case "agent_spawn":
		return "Spawn"
	case "obsidian":
		return "Obsidian"
	case "soul_update":
		return "Update soul"
	case "task_create":
		return "Create task"
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
