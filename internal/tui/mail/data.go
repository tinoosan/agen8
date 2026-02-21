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

type dataLoadedMsg struct {
	inbox     []taskEntry
	outbox    []taskEntry
	current   *taskEntry
	preserve  bool
	connected bool
	err       error
}

type tickMsg struct{}

type sessionSyncedMsg struct {
	sessionID string
	changed   bool
	err       error
}

type taskEntry struct {
	ID           string
	RunID        string
	Role         string
	Goal         string
	Status       string
	Summary      string
	Error        string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
	Artifacts    int
	CreatedAt    time.Time
	CompletedAt  time.Time
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

		// Fetch inbox
		var inboxRes protocol.TaskListResult
		if err := client.Call(ctx, protocol.MethodTaskList, protocol.TaskListParams{
			ThreadID: protocol.ThreadID(scope.ThreadID),
			TeamID:   strings.TrimSpace(scope.TeamID),
			RunID:    strings.TrimSpace(scope.RunID),
			View:     "inbox",
			Limit:    200,
			Offset:   0,
		}, &inboxRes); err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}

		// Fetch outbox
		var outboxRes protocol.TaskListResult
		if err := client.Call(ctx, protocol.MethodTaskList, protocol.TaskListParams{
			ThreadID: protocol.ThreadID(scope.ThreadID),
			TeamID:   strings.TrimSpace(scope.TeamID),
			RunID:    strings.TrimSpace(scope.RunID),
			View:     "outbox",
			Limit:    200,
			Offset:   0,
		}, &outboxRes); err != nil {
			if rpcscope.IsScopeUnavailable(err) {
				return dataLoadedMsg{preserve: true, connected: true, err: err}
			}
			return dataLoadedMsg{err: err}
		}

		inbox := filterTasks(inboxRes.Tasks, true)
		outbox := filterTasks(outboxRes.Tasks, false)

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

func filterTasks(tasks []protocol.Task, isInbox bool) []taskEntry {
	out := make([]taskEntry, 0, len(tasks))
	for _, t := range tasks {
		status := strings.TrimSpace(t.Status)
		if isInbox {
			if status != string(types.TaskStatusPending) && status != string(types.TaskStatusActive) {
				continue
			}
		}
		id := strings.TrimSpace(t.ID)
		if id == "" {
			continue
		}
		out = append(out, taskEntry{
			ID:           id,
			RunID:        strings.TrimSpace(string(t.RunID)),
			Role:         strings.TrimSpace(t.AssignedRole),
			Goal:         strings.TrimSpace(t.Goal),
			Status:       status,
			Summary:      strings.TrimSpace(t.Summary),
			Error:        strings.TrimSpace(t.Error),
			InputTokens:  t.InputTokens,
			OutputTokens: t.OutputTokens,
			TotalTokens:  t.TotalTokens,
			CostUSD:      t.CostUSD,
			Artifacts:    len(t.Artifacts),
			CreatedAt:    t.CreatedAt,
			CompletedAt:  t.CompletedAt,
		})
	}
	return out
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
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
