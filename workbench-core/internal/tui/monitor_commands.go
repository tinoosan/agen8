package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/resources"
)

type commandHandler struct {
	fn                func(m *monitorModel, args string) tea.Cmd
	desc              string
	invokeWithoutArgs bool
}

type monitorCommandSpec struct {
	name              string
	desc              string
	invokeWithoutArgs bool
	fn                func(m *monitorModel, args string) tea.Cmd
}

var monitorCommandSpecs = []monitorCommandSpec{
	{name: "/new", desc: "Start a new session", fn: cmdNewSession, invokeWithoutArgs: true},
	{name: "/reconnect", desc: "Reconnect to daemon", fn: cmdReconnect, invokeWithoutArgs: true},
	{name: "/artifact", desc: "Open artifact viewer", fn: cmdArtifact, invokeWithoutArgs: true},
	{name: "/team", desc: "Focus team run", fn: cmdTeam, invokeWithoutArgs: true},
	{name: "/sessions", desc: "Browse sessions", fn: cmdSessions, invokeWithoutArgs: true},
	{name: "/agents", desc: "Browse agents", fn: cmdAgents, invokeWithoutArgs: true},
	{name: "/rename", desc: "Rename current session", fn: cmdRenameSession},
	{name: "/pause", desc: "Pause run(s)", fn: cmdPause, invokeWithoutArgs: true},
	{name: "/resume", desc: "Resume run(s)", fn: cmdResume, invokeWithoutArgs: true},
	{name: "/stop", desc: "Hard-stop run(s)", fn: cmdStop, invokeWithoutArgs: true},
	{name: "/clear", desc: "Clear persisted run history", fn: cmdClear, invokeWithoutArgs: true},
	{name: "/model", desc: "Set or pick model", fn: cmdModel, invokeWithoutArgs: true},
	{name: "/effort", desc: "Set reasoning effort", fn: cmdReasoningEffort, invokeWithoutArgs: true},
	{name: "/reasoning-summary", desc: "Set reasoning summary", fn: cmdReasoningSummary, invokeWithoutArgs: true},
	{name: "/memory", desc: "Search memory", fn: cmdMemorySearch},
	{name: "/copy", desc: "Copy agent output transcript", fn: cmdCopy, invokeWithoutArgs: true},
	{name: "/editor", desc: "Open external editor", fn: cmdEditor, invokeWithoutArgs: true},
	{name: "/help", desc: "Show help", fn: cmdHelp, invokeWithoutArgs: true},
	{name: "/quit", desc: "Exit monitor", fn: cmdQuit, invokeWithoutArgs: true},
}

var monitorCommands = buildMonitorCommandRegistry(monitorCommandSpecs)

var monitorAvailableCommands = monitorCommandNames(monitorCommandSpecs)

func buildMonitorCommandRegistry(specs []monitorCommandSpec) map[string]commandHandler {
	registry := make(map[string]commandHandler, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.name)
		if name == "" || spec.fn == nil {
			continue
		}
		registry[name] = commandHandler{
			fn:                spec.fn,
			desc:              strings.TrimSpace(spec.desc),
			invokeWithoutArgs: spec.invokeWithoutArgs,
		}
	}
	return registry
}

func monitorCommandNames(specs []monitorCommandSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func monitorCommandDescription(cmd string) string {
	h, ok := monitorCommands[strings.TrimSpace(cmd)]
	if !ok {
		return ""
	}
	return h.desc
}

func isExactMonitorCommand(s string) bool {
	_, ok := monitorCommands[strings.TrimSpace(s)]
	return ok
}

func monitorCommandInvokesWithoutArgs(cmd string) bool {
	h, ok := monitorCommands[strings.TrimSpace(cmd)]
	if !ok {
		return false
	}
	return h.invokeWithoutArgs
}

func (m *monitorModel) handleCommand(raw string) tea.Cmd {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	cmd, rest := splitMonitorCommand(raw)

	if cmd == "" || !strings.HasPrefix(cmd, "/") {
		// Instant user-echo: show the submitted goal immediately so the user
		// doesn't feel like nothing happened while the RPC round-trip runs.
		m.appendAgentOutput("▸ " + truncateText(strings.TrimSpace(raw), 120))
		return m.enqueueTask(strings.TrimSpace(raw), 0)
	}
	handler, ok := monitorCommands[cmd]
	if !ok {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] unknown command: " + cmd}} }
	}
	if handler.fn == nil {
		return nil
	}
	return handler.fn(m, rest)
}

func cmdQuit(m *monitorModel, _ string) tea.Cmd {
	if m != nil && m.cancel != nil {
		m.cancel()
	}
	return tea.Quit
}

func cmdHelp(m *monitorModel, _ string) tea.Cmd {
	m.openHelpModal()
	return nil
}

func cmdReconnect(m *monitorModel, _ string) tea.Cmd {
	m.rpcChecking = true
	return m.checkRPCHealthCmd(true)
}

func cmdEditor(m *monitorModel, _ string) tea.Cmd {
	return m.openComposeEditor("")
}

func cmdCopy(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /copy"}} }
	}
	text := strings.TrimSpace(m.agentOutputForClipboard())
	if text == "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[copy] nothing to copy"}} }
	}
	return copyToClipboardCmd(text)
}

func cmdNewSession(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) == "" {
		return m.openNewSessionWizard()
	}
	req := parseNewSessionRequest(strings.TrimSpace(rest), strings.TrimSpace(m.profile))
	switch req.Mode {
	case "team":
		if strings.TrimSpace(req.Profile) == "" {
			return m.openProfilePickerFor("new-team", true)
		}
		return m.startNewTeamSession(req.Profile, req.Goal)
	case "standalone":
		return m.startNewStandaloneSession(req.Profile, req.Goal)
	default:
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] usage: /new [standalone [profile]] [goal] | /new team <profile> [goal]"}}
		}
	}
}

func cmdRenameSession(m *monitorModel, rest string) tea.Cmd {
	title := strings.TrimSpace(rest)
	if title == "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /rename <title>"}} }
	}
	return func() tea.Msg {
		var res protocol.SessionRenameResult
		if err := m.rpcRoundTrip(protocol.MethodSessionRename, protocol.SessionRenameParams{
			ThreadID:  protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			SessionID: strings.TrimSpace(m.sessionID),
			Title:     title,
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[session] renamed: " + strings.TrimSpace(res.Title)}}
	}
}

func cmdArtifact(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /artifact"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	return m.openArtifactViewer()
}

func cmdTeam(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(m.teamID) == "" {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] /team is only available in team monitor"}}
		}
	}
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /team"}} }
	}
	return m.openTeamPicker()
}

func cmdSessions(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /sessions"}} }
	}
	return m.openSessionPicker()
}

func cmdAgents(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /agents"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] select or create a session first: /new or /sessions"}}
		}
	}
	return m.openAgentPicker()
}

func cmdPause(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /pause"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	return func() tea.Msg {
		if strings.TrimSpace(m.teamID) != "" {
			controlSessionID := strings.TrimSpace(m.resolveTeamControlSessionID())
			if controlSessionID == "" {
				controlSessionID = strings.TrimSpace(m.rpcRun().SessionID)
			}
			var res protocol.SessionPauseResult
			if err := m.rpcRoundTrip(protocol.MethodSessionPause, protocol.SessionPauseParams{
				ThreadID:  protocol.ThreadID(controlSessionID),
				SessionID: controlSessionID,
			}, &res); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "thread not found") {
					if refreshed := strings.TrimSpace(m.resolveTeamControlSessionID()); refreshed != "" && refreshed != controlSessionID {
						if rerr := m.rpcRoundTrip(protocol.MethodSessionPause, protocol.SessionPauseParams{
							ThreadID:  protocol.ThreadID(refreshed),
							SessionID: refreshed,
						}, &res); rerr == nil {
							return commandLinesMsg{lines: []string{fmt.Sprintf("[pause] team paused (%d runs)", len(res.AffectedRunIDs))}}
						}
					}
				}
				return commandLinesMsg{lines: []string{"[pause] error: " + err.Error()}}
			}
			return commandLinesMsg{lines: []string{fmt.Sprintf("[pause] team paused (%d runs)", len(res.AffectedRunIDs))}}
		}
		threadID := protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID))
		var res protocol.AgentPauseResult
		if err := m.rpcRoundTrip(protocol.MethodAgentPause, protocol.AgentPauseParams{
			ThreadID: threadID,
			RunID:    strings.TrimSpace(m.runID),
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[pause] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[pause] run paused: " + shortID(strings.TrimSpace(res.RunID))}}
	}
}

func cmdResume(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /resume"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	return func() tea.Msg {
		if strings.TrimSpace(m.teamID) != "" {
			controlSessionID := strings.TrimSpace(m.resolveTeamControlSessionID())
			if controlSessionID == "" {
				controlSessionID = strings.TrimSpace(m.rpcRun().SessionID)
			}
			var res protocol.SessionResumeResult
			if err := m.rpcRoundTrip(protocol.MethodSessionResume, protocol.SessionResumeParams{
				ThreadID:  protocol.ThreadID(controlSessionID),
				SessionID: controlSessionID,
			}, &res); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "thread not found") {
					if refreshed := strings.TrimSpace(m.resolveTeamControlSessionID()); refreshed != "" && refreshed != controlSessionID {
						if rerr := m.rpcRoundTrip(protocol.MethodSessionResume, protocol.SessionResumeParams{
							ThreadID:  protocol.ThreadID(refreshed),
							SessionID: refreshed,
						}, &res); rerr == nil {
							return commandLinesMsg{lines: []string{fmt.Sprintf("[resume] team resumed (%d runs)", len(res.AffectedRunIDs))}}
						}
					}
				}
				return commandLinesMsg{lines: []string{"[resume] error: " + err.Error()}}
			}
			return commandLinesMsg{lines: []string{fmt.Sprintf("[resume] team resumed (%d runs)", len(res.AffectedRunIDs))}}
		}
		threadID := protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID))
		var res protocol.AgentResumeResult
		if err := m.rpcRoundTrip(protocol.MethodAgentResume, protocol.AgentResumeParams{
			ThreadID: threadID,
			RunID:    strings.TrimSpace(m.runID),
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[resume] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[resume] run resumed: " + shortID(strings.TrimSpace(res.RunID))}}
	}
}

func cmdStop(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /stop"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	return func() tea.Msg {
		controlSessionID := strings.TrimSpace(m.rpcRun().SessionID)
		if strings.TrimSpace(m.teamID) != "" {
			controlSessionID = strings.TrimSpace(m.resolveTeamControlSessionID())
			if controlSessionID == "" {
				controlSessionID = strings.TrimSpace(m.rpcRun().SessionID)
			}
		}
		var res protocol.SessionStopResult
		if err := m.rpcRoundTrip(protocol.MethodSessionStop, protocol.SessionStopParams{
			ThreadID:  protocol.ThreadID(controlSessionID),
			SessionID: controlSessionID,
		}, &res); err != nil {
			if strings.TrimSpace(m.teamID) != "" && strings.Contains(strings.ToLower(err.Error()), "thread not found") {
				if refreshed := strings.TrimSpace(m.resolveTeamControlSessionID()); refreshed != "" && refreshed != controlSessionID {
					if rerr := m.rpcRoundTrip(protocol.MethodSessionStop, protocol.SessionStopParams{
						ThreadID:  protocol.ThreadID(refreshed),
						SessionID: refreshed,
					}, &res); rerr == nil {
						return commandLinesMsg{lines: []string{fmt.Sprintf("[stop] team stopped (%d runs)", len(res.AffectedRunIDs))}}
					}
				}
			}
			return commandLinesMsg{lines: []string{"[stop] error: " + err.Error()}}
		}
		if strings.TrimSpace(m.teamID) != "" {
			return commandLinesMsg{lines: []string{fmt.Sprintf("[stop] team stopped (%d runs)", len(res.AffectedRunIDs))}}
		}
		return commandLinesMsg{lines: []string{fmt.Sprintf("[stop] session stopped (%d runs)", len(res.AffectedRunIDs))}}
	}
}

func cmdClear(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) != "" {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /clear"}} }
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	return m.openHistoryClearConfirm()
}

func cmdModel(m *monitorModel, rest string) tea.Cmd {
	if strings.TrimSpace(rest) == "" {
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		return m.openModelPicker()
	}
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	model := strings.TrimSpace(rest)
	if strings.TrimSpace(m.teamID) != "" {
		return m.writeTeamControl("set_team_model", model)
	}
	return m.writeControl("set_model", map[string]any{"model": model})
}

func cmdReasoningEffort(m *monitorModel, _ string) tea.Cmd {
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	m.openReasoningEffortPicker()
	return nil
}

func cmdReasoningSummary(m *monitorModel, _ string) tea.Cmd {
	if m.isDetached() {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
		}
	}
	m.openReasoningSummaryPicker()
	return nil
}

func cmdMemorySearch(m *monitorModel, rest string) tea.Cmd {
	query := strings.TrimSpace(rest)
	query = strings.Trim(query, "\"")
	return m.searchMemory(query)
}

type newSessionRequest struct {
	Mode    string
	Profile string
	Goal    string
}

func parseNewSessionRequest(rest, defaultProfile string) newSessionRequest {
	rest = strings.TrimSpace(rest)
	defaultProfile = strings.TrimSpace(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = "general"
	}
	if rest == "" {
		return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
	}
	toks := strings.Fields(rest)
	if len(toks) == 0 {
		return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
	}
	mode := strings.ToLower(strings.TrimSpace(toks[0]))
	switch mode {
	case "team":
		if len(toks) == 1 {
			return newSessionRequest{Mode: "team"}
		}
		return newSessionRequest{
			Mode:    "team",
			Profile: strings.TrimSpace(toks[1]),
			Goal:    strings.TrimSpace(strings.Join(toks[2:], " ")),
		}
	case "standalone":
		if len(toks) == 1 {
			return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
		}
		return newSessionRequest{
			Mode:    "standalone",
			Profile: strings.TrimSpace(toks[1]),
			Goal:    strings.TrimSpace(strings.Join(toks[2:], " ")),
		}
	default:
		return newSessionRequest{
			Mode:    "standalone",
			Profile: defaultProfile,
			Goal:    strings.TrimSpace(rest),
		}
	}
}

func (m *monitorModel) startNewStandaloneSession(profileRef, goal string) tea.Cmd {
	return func() tea.Msg {
		var res protocol.SessionStartResult
		if err := m.rpcRoundTrip(protocol.MethodSessionStart, protocol.SessionStartParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Mode:     "standalone",
			Profile:  strings.TrimSpace(profileRef),
			Goal:     strings.TrimSpace(goal),
			Model:    "", // Do not inherit current session's model; let profile resolve it.
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
		}
		runID := strings.TrimSpace(res.PrimaryRunID)
		if runID == "" {
			return commandLinesMsg{lines: []string{"[session] error: session.start returned empty primaryRunId"}}
		}
		return monitorSwitchRunMsg{RunID: runID}
	}
}

func (m *monitorModel) startNewTeamSession(profileRef, goal string) tea.Cmd {
	return func() tea.Msg {
		var res protocol.SessionStartResult
		if err := m.rpcRoundTrip(protocol.MethodSessionStart, protocol.SessionStartParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Mode:     "team",
			Profile:  strings.TrimSpace(profileRef),
			Goal:     strings.TrimSpace(goal),
			Model:    "", // Do not inherit current session's model; let profile resolve it.
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
		}
		teamID := strings.TrimSpace(res.TeamID)
		if teamID == "" {
			return commandLinesMsg{lines: []string{"[session] error: session.start(team) returned empty teamId"}}
		}
		return monitorSwitchTeamMsg{TeamID: teamID}
	}
}

func (m *monitorModel) enqueueTask(goal string, priority int) tea.Cmd {
	return func() tea.Msg {
		if m.isDetached() {
			return commandLinesMsg{lines: []string{"[queued] error: no active context; use /new or /sessions first"}}
		}
		goal = strings.TrimSpace(goal)
		if goal == "" {
			return commandLinesMsg{lines: []string{"[queued] error: goal is empty"}}
		}
		targetRunID, err := m.resolveEnqueueTargetRunID()
		if err != nil {
			if strings.TrimSpace(m.teamID) != "" {
				return commandLinesMsg{lines: []string{"[queued] error: cannot queue: coordinator run unavailable (refresh team manifest or pick an agent)"}}
			}
			return commandLinesMsg{lines: []string{"[queued] error: cannot queue: " + err.Error()}}
		}
		autoResumed, err := m.ensureTargetRunRunning(targetRunID)
		if err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		controlSessionID := strings.TrimSpace(m.resolveSessionIDForRun(targetRunID))
		if controlSessionID == "" {
			controlSessionID = strings.TrimSpace(m.rpcRun().SessionID)
		}
		params := protocol.TaskCreateParams{
			ThreadID:       protocol.ThreadID(controlSessionID),
			Goal:           goal,
			Priority:       priority,
			RunID:          targetRunID,
			AssignedToType: "agent",
			AssignedTo:     targetRunID,
			AssignedRole:   "",
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			targetRole := strings.TrimSpace(m.resolveRoleForRun(targetRunID))
			if targetRole == "" {
				return commandLinesMsg{lines: []string{
					fmt.Sprintf("[queued] error: cannot queue: target role unavailable for run %s; refresh team manifest or pick a valid agent", shortID(targetRunID)),
				}}
			}
			params.AssignedRole = targetRole
			params.AssignedToType = "role"
			params.AssignedTo = targetRole
		}
		var res protocol.TaskCreateResult
		if err := m.rpcRoundTrip(protocol.MethodTaskCreate, params, &res); err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		id := strings.TrimSpace(res.Task.ID)
		if id == "" {
			id = "task-" + uuid.NewString()
		}
		suffix := "run " + targetRunID
		extra := []tea.Cmd{}
		if strings.TrimSpace(m.teamID) != "" {
			suffix = "run " + targetRunID + " (team " + m.teamID + ")"
			extra = append(extra, m.loadTeamStatus())
		}
		lines := make([]string, 0, 2)
		if autoResumed {
			lines = append(lines, "[resume] auto-resumed run: "+shortID(targetRunID))
		}
		lines = append(lines, "[queued] "+id+" "+goal+" — task queued to "+suffix)
		cmds := []tea.Cmd{
			func() tea.Msg {
				return commandLinesMsg{lines: lines}
			},
			func() tea.Msg { return taskQueuedLocallyMsg{TaskID: id, Goal: goal} },
		}
		cmds = append(cmds, extra...)
		return tea.Batch(
			cmds...,
		)
	}
}

func (m *monitorModel) ensureTargetRunRunning(targetRunID string) (bool, error) {
	targetRunID = strings.TrimSpace(targetRunID)
	if targetRunID == "" {
		return false, fmt.Errorf("target run is required")
	}
	sessionID := strings.TrimSpace(m.resolveSessionIDForRun(targetRunID))
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.rpcRun().SessionID)
	}
	var list protocol.AgentListResult
	if err := m.rpcRoundTrip(protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(sessionID),
		SessionID: sessionID,
	}, &list); err != nil {
		return false, fmt.Errorf("resolve run status before queue: %w", err)
	}
	status := ""
	for _, item := range list.Agents {
		if strings.TrimSpace(item.RunID) == targetRunID {
			status = strings.ToLower(strings.TrimSpace(item.Status))
			break
		}
	}
	if status != "paused" {
		return false, nil
	}
	var resumed protocol.AgentResumeResult
	if err := m.rpcRoundTrip(protocol.MethodAgentResume, protocol.AgentResumeParams{
		ThreadID: protocol.ThreadID(sessionID),
		RunID:    targetRunID,
	}, &resumed); err != nil {
		return false, fmt.Errorf("auto-resume before queue failed: %w", err)
	}
	return true, nil
}

func (m *monitorModel) writeControl(command string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "set_model":
			model := ""
			if args != nil {
				if v, ok := args["model"].(string); ok {
					model = strings.TrimSpace(v)
				}
			}
			if model == "" {
				return commandLinesMsg{lines: []string{"[control] error: model is required"}}
			}
			var res protocol.ControlSetModelResult
			if err := m.rpcRoundTrip(protocol.MethodControlSetModel, protocol.ControlSetModelParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Model:    model,
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
			}
			m.model = model
			return commandLinesMsg{lines: []string{"[control] applied set_model -> " + model}}
		case "set_reasoning":
			effort := ""
			summary := ""
			if args != nil {
				if v, ok := args["effort"].(string); ok {
					effort = strings.ToLower(strings.TrimSpace(v))
				}
				if v, ok := args["summary"].(string); ok {
					summary = strings.ToLower(strings.TrimSpace(v))
				}
			}
			if summary == "none" {
				summary = "off"
			}
			if effort == "" && summary == "" {
				return commandLinesMsg{lines: []string{"[control] error: effort or summary is required"}}
			}
			var res protocol.ControlSetReasoningResult
			if err := m.rpcRoundTrip(protocol.MethodControlSetReasoning, protocol.ControlSetReasoningParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Effort:   effort,
				Summary:  summary,
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
			}
			parts := make([]string, 0, 2)
			if v := strings.TrimSpace(res.Effort); v != "" {
				parts = append(parts, "effort="+v)
			} else if effort != "" {
				parts = append(parts, "effort="+effort)
			}
			if v := strings.TrimSpace(res.Summary); v != "" {
				parts = append(parts, "summary="+v)
			} else if summary != "" {
				parts = append(parts, "summary="+summary)
			}
			if len(parts) == 0 {
				parts = append(parts, "updated")
			}
			return commandLinesMsg{lines: []string{"[control] applied set_reasoning -> " + strings.Join(parts, ", ")}}
		default:
			return commandLinesMsg{lines: []string{"[control] error: unsupported command " + command}}
		}
	}
}

func (m *monitorModel) writeTeamControl(command, model string) tea.Cmd {
	return func() tea.Msg {
		teamID := strings.TrimSpace(m.teamID)
		if teamID == "" {
			return commandLinesMsg{lines: []string{"[control] error: team id is required"}}
		}
		command = strings.TrimSpace(command)
		model = strings.TrimSpace(model)
		if command == "" || model == "" {
			return commandLinesMsg{lines: []string{"[control] error: command and model are required"}}
		}
		var res protocol.ControlSetModelResult
		if err := m.rpcRoundTrip(protocol.MethodControlSetModel, protocol.ControlSetModelParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Model:    model,
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		m.model = model
		return tea.Batch(
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[control] queued team model change -> " + model}}
			},
			m.loadTeamManifestCmd(),
		)
	}
}

func (m *monitorModel) queueTeamModelChange(model, target, reason string) ([]string, error) {
	teamID := strings.TrimSpace(m.teamID)
	if teamID == "" {
		return nil, fmt.Errorf("team id is required")
	}
	manifestPath := filepath.Join(fsutil.GetTeamDir(m.cfg.DataDir, teamID), "team.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest teamManifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	manifest.ModelChange = &teamModelChangeFile{
		RequestedModel: strings.TrimSpace(model),
		Status:         "pending",
		RequestedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Reason:         strings.TrimSpace(reason),
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, b, 0o644); err != nil {
		return nil, err
	}
	if strings.TrimSpace(target) != "" {
		return []string{strings.TrimSpace(target)}, nil
	}
	return []string{}, nil
}

func (m *monitorModel) searchMemory(query string) tea.Cmd {
	return func() tea.Msg {
		query = strings.TrimSpace(query)
		if query == "" {
			return commandLinesMsg{lines: []string{"[memory] error: query is empty"}}
		}
		memDir := fsutil.GetMemoryDir(m.cfg.DataDir)
		res, err := resources.NewDailyMemoryResource(memDir)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		results, err := res.Search(m.ctx, "", query, 5)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		lines := []string{"[memory] search: " + query}
		for _, r := range results {
			lines = append(lines, fmt.Sprintf("  - %.3f %s (%s)", r.Score, r.Title, r.Path))
		}
		return commandLinesMsg{lines: lines}
	}
}
