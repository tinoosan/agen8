package mail

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
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
	inbox     []taskEntry
	outbox    []taskEntry
	current   *taskEntry
	preserve  bool
	connected bool
	err       error
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

type taskEntry struct {
	ID              string
	MessageID       string
	RunID           string
	Role            string
	Goal            string
	Status          string
	MessageStatus   string
	Kind            string
	Channel         string
	DisplayStatus   string
	Source          string
	Summary         string
	Subject         string
	BodyPreview     string
	Error           string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	CostUSD         float64
	Artifacts       int
	ReadOnly        bool
	CanClaim        bool
	CanComplete     bool
	CreatedAt       time.Time
	CompletedAt     time.Time
	BatchMode       bool
	BatchSynthetic  bool
	BatchDelivered  bool
	BatchParentID   string
	BatchWaveID     string
	BatchIncludedIn string
	IsBatchGroup    bool
	Expanded        bool
	Children        []taskEntry
}

func fetchDataCmd(endpoint, sessionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client := rpcscope.NewClient(endpoint, sessionID).WithTimeout(5 * time.Second)
		scope, err := client.RefreshScope(ctx)
		if err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}
		if strings.TrimSpace(scope.TeamID) == "" && strings.TrimSpace(scope.RunID) == "" {
			return dataLoadedMsg{preserve: true, connected: true, err: fmt.Errorf("%w: missing run/team scope", rpcscope.ErrScopeUnavailable)}
		}

		var inboxRes protocol.MessageListResult
		scopeMode := "run"
		runID := strings.TrimSpace(scope.RunID)
		if strings.TrimSpace(scope.TeamID) != "" {
			scopeMode = "team"
			runID = ""
		}
		if err := client.Call(ctx, protocol.MethodMessageList, protocol.MessageListParams{
			ThreadID: protocol.ThreadID(scope.ThreadID),
			Scope:    scopeMode,
			TeamID:   strings.TrimSpace(scope.TeamID),
			RunID:    runID,
			View:     "inbox",
			Limit:    200,
			Offset:   0,
		}, &inboxRes); err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}

		var outboxRes protocol.MessageListResult
		if err := client.Call(ctx, protocol.MethodMessageList, protocol.MessageListParams{
			ThreadID: protocol.ThreadID(scope.ThreadID),
			Scope:    scopeMode,
			TeamID:   strings.TrimSpace(scope.TeamID),
			RunID:    runID,
			View:     "outbox",
			Limit:    200,
			Offset:   0,
		}, &outboxRes); err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}

		inbox := filterMessages(inboxRes.Messages, true)
		outbox := filterMessages(outboxRes.Messages, false)

		var current *taskEntry
		for i := range inbox {
			if inbox[i].Status == string(types.TaskStatusActive) {
				current = &inbox[i]
				break
			}
		}

		return dataLoadedMsg{
			inbox:     inbox,
			outbox:    outbox,
			current:   current,
			connected: true,
		}
	}
}

func filterMessages(messages []protocol.MailMessage, isInbox bool) []taskEntry {
	flat := make([]taskEntry, 0, len(messages))
	for _, msg := range messages {
		entry := taskEntryFromMessage(msg)
		if entry.ID == "" {
			continue
		}
		if isInbox && !matchesInbox(entry) {
			continue
		}
		if !isInbox && !matchesOutbox(entry) {
			continue
		}
		flat = append(flat, entry)
	}
	return buildTaskProjection(flat)
}

func taskEntryFromMessage(msg protocol.MailMessage) taskEntry {
	t := msg.Task
	if t == nil {
		status := strings.TrimSpace(msg.Status)
		return taskEntry{
			ID:            strings.TrimSpace(msg.MessageID),
			MessageID:     strings.TrimSpace(msg.MessageID),
			RunID:         strings.TrimSpace(string(msg.RunID)),
			Goal:          firstNonEmpty(strings.TrimSpace(msg.Subject), strings.TrimSpace(msg.Summary), strings.TrimSpace(msg.BodyPreview)),
			Status:        status,
			MessageStatus: status,
			Kind:          strings.TrimSpace(msg.Kind),
			Channel:       strings.TrimSpace(msg.Channel),
			DisplayStatus: status,
			Summary:       strings.TrimSpace(msg.Summary),
			Subject:       strings.TrimSpace(msg.Subject),
			BodyPreview:   strings.TrimSpace(msg.BodyPreview),
			Error:         strings.TrimSpace(msg.Error),
			ReadOnly:      msg.ReadOnly,
			CanClaim:      msg.CanClaim,
			CanComplete:   msg.CanComplete,
			CreatedAt:     msg.CreatedAt,
			CompletedAt:   derefTime(msg.ProcessedAt),
		}
	}
	return taskEntry{
		ID:              strings.TrimSpace(t.ID),
		MessageID:       strings.TrimSpace(msg.MessageID),
		RunID:           strings.TrimSpace(string(t.RunID)),
		Role:            firstNonEmpty(strings.TrimSpace(t.AssignedRole), strings.TrimSpace(t.RoleSnapshot)),
		Goal:            strings.TrimSpace(t.Goal),
		Status:          strings.TrimSpace(t.Status),
		MessageStatus:   strings.TrimSpace(msg.Status),
		Kind:            strings.TrimSpace(msg.Kind),
		Channel:         strings.TrimSpace(msg.Channel),
		DisplayStatus:   strings.TrimSpace(t.Status),
		Source:          strings.TrimSpace(t.Source),
		Summary:         strings.TrimSpace(t.Summary),
		Subject:         strings.TrimSpace(msg.Subject),
		BodyPreview:     strings.TrimSpace(msg.BodyPreview),
		Error:           firstNonEmpty(strings.TrimSpace(t.Error), strings.TrimSpace(msg.Error)),
		InputTokens:     t.InputTokens,
		OutputTokens:    t.OutputTokens,
		TotalTokens:     t.TotalTokens,
		CostUSD:         t.CostUSD,
		Artifacts:       len(t.Artifacts),
		ReadOnly:        msg.ReadOnly,
		CanClaim:        msg.CanClaim,
		CanComplete:     msg.CanComplete,
		CreatedAt:       t.CreatedAt,
		CompletedAt:     t.CompletedAt,
		BatchMode:       t.BatchMode,
		BatchSynthetic:  t.BatchSynthetic,
		BatchDelivered:  t.BatchDelivered,
		BatchParentID:   strings.TrimSpace(t.BatchParentTaskID),
		BatchWaveID:     strings.TrimSpace(t.BatchWaveID),
		BatchIncludedIn: strings.TrimSpace(t.BatchIncludedIn),
	}
}

func derefTime(ts *time.Time) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.UTC()
}

func matchesInbox(entry taskEntry) bool {
	if entry.ReadOnly {
		switch entry.MessageStatus {
		case types.MessageStatusPending, types.MessageStatusClaimed, types.MessageStatusNacked:
			return true
		default:
			return false
		}
	}
	switch entry.Status {
	case string(types.TaskStatusPending), string(types.TaskStatusActive), string(types.TaskStatusReviewPending):
		return true
	default:
		return false
	}
}

func matchesOutbox(entry taskEntry) bool {
	if entry.ReadOnly {
		return entry.Channel == types.MessageChannelOutbox || entry.MessageStatus == types.MessageStatusAcked || entry.MessageStatus == types.MessageStatusDeadletter
	}
	switch entry.Status {
	case string(types.TaskStatusReviewPending), string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled):
		return true
	default:
		return false
	}
}

func buildTaskProjection(entries []taskEntry) []taskEntry {
	topLevel := make([]taskEntry, 0, len(entries))
	parentIndex := map[string]int{}
	stagedByParent := map[string][]taskEntry{}
	orphanChildren := make([]taskEntry, 0)

	for _, entry := range entries {
		if entry.ID == "" {
			continue
		}
		if isStagedBatchChild(entry) {
			parentID := strings.TrimSpace(entry.BatchIncludedIn)
			if parentID == "" {
				orphanChildren = append(orphanChildren, entry)
				continue
			}
			stagedByParent[parentID] = append(stagedByParent[parentID], entry)
			continue
		}
		if entry.DisplayStatus == "" {
			entry.DisplayStatus = entry.Status
		}
		topLevel = append(topLevel, entry)
		parentIndex[entry.ID] = len(topLevel) - 1
	}

	for parentID, children := range stagedByParent {
		idx, ok := parentIndex[parentID]
		if !ok {
			orphanChildren = append(orphanChildren, children...)
			continue
		}
		parent := topLevel[idx]
		for i := range children {
			if children[i].DisplayStatus == "" {
				children[i].DisplayStatus = children[i].Status
			}
			if strings.EqualFold(strings.TrimSpace(children[i].Status), string(types.TaskStatusReviewPending)) && isTerminalTaskStatus(parent.Status) {
				children[i].DisplayStatus = "batched"
			}
		}
		parent.Children = append(parent.Children, children...)
		parent.IsBatchGroup = len(parent.Children) > 0
		topLevel[idx] = parent
	}

	for _, orphan := range orphanChildren {
		if orphan.DisplayStatus == "" {
			orphan.DisplayStatus = orphan.Status
		}
		topLevel = append(topLevel, orphan)
	}

	return topLevel
}

func isStagedBatchChild(entry taskEntry) bool {
	return entry.BatchMode && !entry.BatchSynthetic && strings.TrimSpace(entry.BatchIncludedIn) != ""
}

func isTerminalTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled), "cancelled":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
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
