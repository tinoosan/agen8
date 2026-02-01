package layout

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func testStyle() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
}

func TestPanelSpecReservesChrome(t *testing.T) {
	style := testStyle()
	mgr := NewManager(style, true)

	grid := mgr.Calculate(120, 50, 5, 6, 6)
	spec := grid.ActivityFeed

	frameW, frameH := style.GetFrameSize()
	if spec.FrameWidth != frameW || spec.FrameHeight != frameH {
		t.Fatalf("expected frame sizes %dx%d, got %dx%d", frameW, frameH, spec.FrameWidth, spec.FrameHeight)
	}
	if want := spec.Width - frameW; want != spec.ContentWidth {
		t.Fatalf("content width mismatch: got %d want %d", spec.ContentWidth, want)
	}
	if want := spec.Height - frameH - spec.TitleHeight; want != spec.ContentHeight {
		t.Fatalf("content height mismatch: got %d want %d", spec.ContentHeight, want)
	}
	if spec.ContentHeight <= 0 {
		t.Fatalf("expected positive content height, got %d", spec.ContentHeight)
	}
}

func TestColumnsShareMainHeight(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	grid := mgr.Calculate(120, 40, 5, 6, 6)

	leftTotal := grid.ActivityFeed.Height + grid.AgentOutput.Height
	rightTotal := grid.ActivityDetail.Height + grid.CurrentTask.Height + grid.Plan.Height + grid.TaskQueue.Height + grid.Stats.Height
	if leftTotal != rightTotal {
		t.Fatalf("left and right column heights differ: left=%d right=%d", leftTotal, rightTotal)
	}

	headerH := 1
	total := leftTotal + grid.Outbox.Height + grid.Memory.Height + grid.Composer.Height + headerH
	if total > grid.ScreenHeight {
		t.Fatalf("layout exceeds screen height: total=%d screen=%d", total, grid.ScreenHeight)
	}
}

func TestSmallTerminalKeepsUsableContent(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	grid := mgr.Calculate(80, 24, 5, 6, 6)

	panels := []PanelSpec{
		grid.ActivityFeed, grid.AgentOutput,
		grid.ActivityDetail, grid.CurrentTask, grid.Plan, grid.TaskQueue, grid.Stats,
		grid.Outbox, grid.Memory, grid.Composer,
	}
	for i, p := range panels {
		if p.ContentWidth < 0 || p.ContentHeight < 0 {
			t.Fatalf("panel %d has negative content size: %dx%d", i, p.ContentWidth, p.ContentHeight)
		}
		if p.Height < 3 {
			t.Fatalf("panel %d height too small: %d", i, p.Height)
		}
	}
}

func TestDynamicPanelHeights(t *testing.T) {
	mgr := NewManager(testStyle(), true)

	// Hidden panels.
	grid := mgr.Calculate(120, 50, 5, 0, 0)
	if grid.Outbox.Height != 0 {
		t.Fatalf("expected hidden outbox, got height %d", grid.Outbox.Height)
	}
	if grid.Memory.Height != 0 {
		t.Fatalf("expected hidden memory, got height %d", grid.Memory.Height)
	}

	// Visible panels.
	grid2 := mgr.Calculate(120, 50, 5, 6, 6)
	if grid2.Outbox.Height != 6 {
		t.Fatalf("expected outbox height 6, got %d", grid2.Outbox.Height)
	}

	mainArea1 := grid.ActivityFeed.Height + grid.AgentOutput.Height
	mainArea2 := grid2.ActivityFeed.Height + grid2.AgentOutput.Height
	if mainArea1 <= mainArea2 {
		t.Fatalf("expected main area to expand when panels hidden: got %d vs %d", mainArea1, mainArea2)
	}
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
	leftTotal := grid.AgentOutput.Height + grid.Outbox.Height
	if leftTotal > mainH {
		t.Fatalf("left column height %d exceeds main height %d", leftTotal, mainH)
	}
	if grid.SidePanel.Height > mainH {
		t.Fatalf("side panel height %d exceeds main height %d", grid.SidePanel.Height, mainH)
	}
	totalH := reserved + mainH
	if totalH > grid.ScreenHeight {
		t.Fatalf("layout total %d exceeds screen height %d", totalH, grid.ScreenHeight)
	}
}

func TestCalculateDashboard_SidePanelHasContentRoom(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	grid := mgr.CalculateDashboard(120, 35, 4, 6, 1)

	if grid.SidePanel.ContentHeight < 5 {
		t.Fatalf("side panel ContentHeight want >= 5 (for tab content), got %d", grid.SidePanel.ContentHeight)
	}
}

func TestCalculateDashboard_OutboxComposerLeftColumnOnly(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	grid := mgr.CalculateDashboard(120, 35, 4, 6, 1)

	leftW := grid.AgentOutput.Width
	if grid.Outbox.Width != leftW {
		t.Fatalf("outbox width want %d (left column), got %d", leftW, grid.Outbox.Width)
	}
	if grid.Composer.Width != leftW {
		t.Fatalf("composer width want %d (left column), got %d", leftW, grid.Composer.Width)
	}
	if grid.Outbox.Width >= grid.ScreenWidth || grid.Composer.Width >= grid.ScreenWidth {
		t.Fatalf("outbox and composer must be left-column only (width < screen width %d)", grid.ScreenWidth)
	}
}

func TestCalculateCompact_100x30_NoClipping(t *testing.T) {
	mgr := NewManager(testStyle(), true)
	composerHeight := 4
	grid := mgr.CalculateCompact(100, 30, composerHeight)

	reserved := 1 + composerHeight
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
