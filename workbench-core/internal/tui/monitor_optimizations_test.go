package tui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAppendThinkingEntry_TrimsToMax(t *testing.T) {
	t.Parallel()

	m := &monitorModel{}
	for i := 0; i < maxThinkingEntries+10; i++ {
		m.appendThinkingEntry("", "", fmt.Sprintf("s-%d", i))
	}

	if len(m.thinkingEntries) != maxThinkingEntries {
		t.Fatalf("len(thinkingEntries)=%d, want %d", len(m.thinkingEntries), maxThinkingEntries)
	}
	if got := m.thinkingEntries[0].Summary; got != "s-10" {
		t.Fatalf("oldest entry=%q, want %q", got, "s-10")
	}
	if got := m.thinkingEntries[len(m.thinkingEntries)-1].Summary; got != fmt.Sprintf("s-%d", maxThinkingEntries+9) {
		t.Fatalf("newest entry=%q, want %q", got, fmt.Sprintf("s-%d", maxThinkingEntries+9))
	}
	if !m.dirtyThinking {
		t.Fatalf("dirtyThinking=false, want true")
	}
}

func TestRefreshViewports_DoesNotClearDirtyThinkingWhenHidden(t *testing.T) {
	t.Parallel()

	in := textarea.New()
	in.SetHeight(2)

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 0, 0)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	m := &monitorModel{
		width:          160,
		height:         48,
		styles:         defaultMonitorStyles(),
		input:          in,
		renderer:       newContentRenderer(),
		activityList:   activityList,
		agentOutputVP:  viewport.New(0, 0),
		outboxVP:       viewport.New(0, 0),
		inboxVP:        viewport.New(0, 0),
		memoryVP:       viewport.New(0, 0),
		planViewport:   viewport.New(0, 0),
		thinkingVP:     viewport.New(0, 0),
		activityDetail: viewport.New(0, 0),
		// Ensure Thoughts are not visible (dashboardSideTab != 3).
		dashboardSideTab: 0,
	}

	m.appendThinkingEntry("", "", "hello")
	if !m.dirtyThinking {
		t.Fatalf("dirtyThinking=false, want true before refresh")
	}

	m.refreshViewports()

	if !m.dirtyThinking {
		t.Fatalf("dirtyThinking=false after refresh while hidden; want it to remain true until visible")
	}
}

func TestScheduleUIRefresh_Debounces(t *testing.T) {
	t.Parallel()

	m := &monitorModel{uiRefreshDebounce: 0}
	cmd1 := m.scheduleUIRefresh()
	if cmd1 == nil {
		t.Fatalf("first scheduleUIRefresh() cmd=nil, want non-nil")
	}
	if !m.uiRefreshScheduled {
		t.Fatalf("uiRefreshScheduled=false, want true")
	}

	cmd2 := m.scheduleUIRefresh()
	if cmd2 != nil {
		t.Fatalf("second scheduleUIRefresh() cmd!=nil, want nil (debounced)")
	}
}

func TestActivityTail_DoesNotReenableOnRefreshWhenUserScrolledUp(t *testing.T) {
	t.Parallel()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 80, 20)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	m := &monitorModel{
		activityList:          activityList,
		activityPageSize:      200,
		activityPage:          0,
		activityTotalCount:    3,
		activityFollowingTail: true,
		activityPageItems: []Activity{
			{ID: "a1", Title: "one"},
			{ID: "a2", Title: "two"},
			{ID: "a3", Title: "three"},
		},
	}
	m.refreshActivityList()
	if !m.activityFollowingTail {
		t.Fatalf("expected tail-follow true after initial refresh")
	}

	m.focusedPanel = panelActivity
	_, _ = m.routeKeyToFocusedPanel(tea.KeyMsg{Type: tea.KeyUp})
	if m.activityFollowingTail {
		t.Fatalf("expected manual scroll to disable tail-follow")
	}

	// Simulate background refresh while user is browsing history.
	m.activityPageItems = []Activity{
		{ID: "a2", Title: "two"},
		{ID: "a3", Title: "three"},
		{ID: "a4", Title: "four"},
	}
	m.activityTotalCount = 4
	m.refreshActivityList()
	if m.activityFollowingTail {
		t.Fatalf("tail-follow was re-enabled by refresh; expected false")
	}
}

func TestActivityScrollIntent_DisablesTailFollow(t *testing.T) {
	t.Parallel()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 80, 20)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	m := &monitorModel{
		activityList:          activityList,
		activityPageSize:      200,
		activityPage:          0,
		activityTotalCount:    2,
		activityFollowingTail: true,
		activityPageItems: []Activity{
			{ID: "b1", Title: "one"},
			{ID: "b2", Title: "two"},
		},
		focusedPanel: panelActivity,
	}
	m.refreshActivityList()
	if !m.activityFollowingTail {
		t.Fatalf("expected initial tail-follow true")
	}

	_, _ = m.routeKeyToFocusedPanel(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.activityFollowingTail {
		t.Fatalf("expected tail-follow false after scroll input")
	}
}
