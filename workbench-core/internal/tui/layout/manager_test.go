package layout

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func testStyle() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
}

func TestCalculateDashboard_120x35_NoClipping(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	composerHeight := 4
	outboxHeight := 6
	grid := mgr.CalculateDashboard(120, 35, composerHeight, outboxHeight, 1)

	reserved := DashHeaderHeight + DashStatusBarHeight + DashWarningHeight + composerHeight + DashGapAfterHeader + DashGapBeforeComposer
	mainH := 35 - reserved
	if mainH < 1 {
		mainH = 1
	}
	if grid.AgentOutput.Height > mainH {
		t.Fatalf("left column height %d exceeds main height %d", grid.AgentOutput.Height, mainH)
	}
	// Right column now renders per-tab panels directly; reserve 1 row for the tab bar.
	if got := grid.ActivityFeed.Height + grid.ActivityDetail.Height + 1; got > mainH {
		t.Fatalf("activity panels height %d exceeds main height %d", got, mainH)
	}
	if got := grid.Plan.Height + 1; got > mainH {
		t.Fatalf("plan panel height %d exceeds main height %d", got, mainH)
	}
	sideContentH := mainH - 1
	if sideContentH < 0 {
		sideContentH = 0
	}
	if got := grid.CurrentTask.Height + grid.Inbox.Height + grid.Outbox.Height; got > sideContentH {
		t.Fatalf("tasks panels height %d exceeds side content height %d", got, sideContentH)
	}
	if grid.Stats.Height != composerHeight {
		t.Fatalf("stats height want %d, got %d", composerHeight, grid.Stats.Height)
	}
	totalH := reserved + mainH
	if totalH > grid.ScreenHeight {
		t.Fatalf("layout total %d exceeds screen height %d", totalH, grid.ScreenHeight)
	}
}

func TestCalculateDashboard_ActivityPanelsHaveContentRoom(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	grid := mgr.CalculateDashboard(120, 35, 4, 6, 1)

	if grid.ActivityFeed.ContentHeight < 3 {
		t.Fatalf("activity feed ContentHeight want >= 3, got %d", grid.ActivityFeed.ContentHeight)
	}
	if grid.ActivityDetail.ContentHeight < 3 {
		t.Fatalf("activity detail ContentHeight want >= 3, got %d", grid.ActivityDetail.ContentHeight)
	}
}

func TestCalculateDashboard_ComposerStatsAlignment(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	composerHeight := 4
	grid := mgr.CalculateDashboard(120, 35, composerHeight, 6, 1)

	leftW := grid.AgentOutput.Width
	if grid.Composer.Width != leftW {
		t.Fatalf("composer width want %d (left column), got %d", leftW, grid.Composer.Width)
	}
	if grid.Stats.Width != grid.ActivityFeed.Width {
		t.Fatalf("stats width want %d (right column), got %d", grid.ActivityFeed.Width, grid.Stats.Width)
	}
	if grid.Stats.Height != composerHeight {
		t.Fatalf("stats height want %d, got %d", composerHeight, grid.Stats.Height)
	}
}

func TestCalculateCompact_100x30_NoClipping(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	composerHeight := 4
	grid := mgr.CalculateCompact(100, 30, composerHeight)

	reserved := 2 + composerHeight
	mainH := 30 - reserved
	if grid.AgentOutput.Height > mainH {
		t.Fatalf("compact main area height %d exceeds remaining %d", grid.AgentOutput.Height, mainH)
	}
	if grid.Composer.Height != composerHeight {
		t.Fatalf("composer height want %d, got %d", composerHeight, grid.Composer.Height)
	}
	totalH := reserved + grid.AgentOutput.Height
	if totalH > grid.ScreenHeight {
		t.Fatalf("compact layout total %d exceeds screen height %d", totalH, grid.ScreenHeight)
	}
}

func TestCalculateDashboard_NarrowWidth_80Cols(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	// 80 cols is < 93 (minLeft 60 + minRight 32 + gap 1).
	// Should split fluidly.
	grid := mgr.CalculateDashboard(80, 35, 4, 6, 1)

	// Verify total width matches screen width exactly (no overflow).
	totalW := grid.AgentOutput.Width + 1 + grid.ActivityFeed.Width // 1 for gap
	if totalW != 80 {
		t.Fatalf("expected total width 80, got %d (left %d, right %d)", totalW, grid.AgentOutput.Width, grid.ActivityFeed.Width)
	}

	// Verify no fixed minimums enforced (Left would be 60 if enforced).
	if grid.AgentOutput.Width >= 60 {
		t.Fatalf("expected fluid left column < 60, got %d", grid.AgentOutput.Width)
	}

	// Check for usability (at least 1 col).
	if grid.AgentOutput.Width < 1 || grid.ActivityFeed.Width < 1 {
		t.Fatalf("columns too small: left %d, right %d", grid.AgentOutput.Width, grid.ActivityFeed.Width)
	}
}
