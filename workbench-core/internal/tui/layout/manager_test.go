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

	grid := mgr.Calculate(120, 50, 5)
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
	grid := mgr.Calculate(120, 40, 5)

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
	grid := mgr.Calculate(80, 24, 5)

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
