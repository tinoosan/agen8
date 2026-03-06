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

type dataLoadedMsg struct {
	activities []types.Activity
	totalCount int
	connected  bool
	preserve   bool
	err        error
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
