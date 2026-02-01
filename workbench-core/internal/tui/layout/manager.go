package layout

import (
	"math"

	"github.com/charmbracelet/lipgloss"
)

// PanelSpec describes both the outer box size and the usable inner content size
// for a dashboard panel. ContentHeight excludes frame (border + padding) and
// any section title chrome so viewport sizing can rely on it directly.
type PanelSpec struct {
	Width         int
	Height        int
	ContentWidth  int
	ContentHeight int
	FrameWidth    int
	FrameHeight   int
	TitleHeight   int
}

// InnerHeight returns the content-box height for lipgloss (total - frame) so
// that total rendered height equals spec.Height and no clipping occurs.
func (p PanelSpec) InnerHeight() int { return max(0, p.Height-p.FrameHeight) }

// InnerWidth returns the content-box width for lipgloss (total - frame) so
// that total rendered width equals spec.Width.
func (p PanelSpec) InnerWidth() int { return max(0, p.Width-p.FrameWidth) }

// GridLayout contains the computed panel specs for the monitor dashboard.
type GridLayout struct {
	ScreenWidth  int
	ScreenHeight int

	// SidePanel is the single right-column tabbed panel (Activity | Plan | Tasks).
	SidePanel      PanelSpec
	ActivityFeed   PanelSpec
	AgentOutput    PanelSpec
	ActivityDetail PanelSpec
	CurrentTask    PanelSpec
	Plan           PanelSpec
	TaskQueue      PanelSpec
	Stats          PanelSpec
	Outbox         PanelSpec
	Memory         PanelSpec
	Composer       PanelSpec
}

// Manager centralizes layout calculations so rendering and viewport sizing share
// a single source of truth.
type Manager struct {
	style       lipgloss.Style
	frameW      int
	frameH      int
	titleHeight int
}

// NewManager builds a Manager using the provided panel style. If hasTitle is
// true, a single-line section title is reserved in the content height budget.
func NewManager(style lipgloss.Style, hasTitle bool) *Manager {
	frameW, frameH := style.GetFrameSize()
	titleH := 0
	if hasTitle {
		// Titles in the monitor are single-line; reserving one row keeps content
		// viewports from overlapping the header.
		titleH = 1
	}
	return &Manager{style: style, frameW: frameW, frameH: frameH, titleHeight: titleH}
}

// CalculateCompact produces a GridLayout for compact mode (single main area + composer).
// Reserves headerHeight (1), composerHeight; gives the rest to AgentOutput full width.
// Other panels get zero height so they are not shown.
func (m *Manager) CalculateCompact(width, height, composerHeight int) GridLayout {
	// Compact view renders: header + tab bar + main panels + composer.
	// Reserve 2 rows so main panels never overflow vertically.
	const headerHeight = 2
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if composerHeight < 0 {
		composerHeight = 0
	}
	mainH := height - headerHeight - composerHeight
	if mainH < 1 {
		mainH = 1
	}
	grid := GridLayout{ScreenWidth: width, ScreenHeight: height}
	grid.AgentOutput = m.spec(width, mainH)
	grid.Composer = m.spec(width, composerHeight)
	// Compact mode still needs per-tab panel specs so each tab can render its own
	// box(es) without overflowing horizontally.
	feedH := int(math.Round(float64(mainH) * 0.40))
	if feedH < 6 {
		feedH = 6
	}
	if feedH > mainH-6 {
		feedH = max(1, mainH-6)
	}
	detailH := mainH - feedH
	if detailH < 1 {
		detailH = 1
	}
	grid.SidePanel = m.spec(width, 0)
	grid.ActivityFeed = m.spec(width, feedH)
	grid.ActivityDetail = m.spec(width, detailH)
	grid.CurrentTask = m.spec(width, 0)
	grid.Plan = m.spec(width, mainH)
	grid.TaskQueue = m.spec(width, 0)
	grid.Stats = m.spec(width, 0)
	grid.Outbox = m.spec(width, mainH)
	grid.Memory = m.spec(width, 0)
	return grid
}

// Dashboard reserved row constants: used by CalculateDashboard so total view fits.
const (
	DashHeaderHeight      = 1
	DashStatusBarHeight   = 1
	DashWarningHeight     = 1
	DashGapAfterHeader    = 1
	DashGapBeforeComposer = 1
)

// CalculateDashboard produces a GridLayout for dashboard mode (two columns).
// Left: AgentOutput (flex) + Outbox (fixed). Right: single SidePanel (tabbed Activity | Plan | Tasks).
// Outbox and Composer use left column width only.
func (m *Manager) CalculateDashboard(width, height, composerHeight, outboxHeight, statusBarHeight int) GridLayout {
	const (
		minLeftWidth  = 60
		minRightWidth = 32
		gapCols       = 1
		outboxFixedH  = 6
		outboxMinH    = 4
	)
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if composerHeight < 0 {
		composerHeight = 0
	}
	if outboxHeight < 0 {
		outboxHeight = 0
	}
	reserved := DashHeaderHeight + max(1, statusBarHeight) + DashWarningHeight + composerHeight + DashGapAfterHeader + DashGapBeforeComposer
	remainingHeight := height - reserved
	if remainingHeight < 1 {
		remainingHeight = 1
	}

	minTotalWidth := minLeftWidth + minRightWidth + gapCols
	var leftW, rightW int

	if width < minTotalWidth {
		// Fluid mode: terminal is too narrow for preferred minimums.
		// Split proportionally: ~66% left, ~33% right.
		// Ensure gap is respected.
		available := width - gapCols
		if available < 0 {
			available = 0
		}
		leftW = int(math.Round(float64(available) * 0.66))
		rightW = available - leftW
		// Ensure at least 1 col each if possible
		if leftW < 1 && available >= 2 {
			leftW = 1
			rightW = available - 1
		} else if rightW < 1 && available >= 2 {
			rightW = 1
			leftW = available - 1
		}
	} else {
		// Standard mode: enforce minimums
		leftW = int(math.Round(float64(width) * 0.66))
		if leftW < minLeftWidth {
			leftW = minLeftWidth
		}
		if leftW > width-minRightWidth-gapCols {
			leftW = max(0, width-minRightWidth-gapCols)
		}
		// Strict constraint: Right width is whatever is left.
		// This guarantees leftW + gap + rightW == width.
		rightW = width - leftW - gapCols
		// If strict math pushes rightW below minimum (shouldn't happen if leftW logic is correct, but safe guard),
		// we prioritize total width > minimums to avoid overflow.
		if rightW < 0 {
			rightW = 0
		}
	}

	outboxH := outboxHeight
	if outboxH > 0 && outboxH < outboxMinH {
		outboxH = outboxMinH
	}
	if outboxH > outboxFixedH {
		outboxH = outboxFixedH
	}
	mainH := remainingHeight
	agentOutputH := mainH - outboxH
	if agentOutputH < 1 {
		agentOutputH = 1
		outboxH = mainH - 1
	}

	grid := GridLayout{ScreenWidth: width, ScreenHeight: height}
	grid.AgentOutput = m.spec(leftW, agentOutputH)
	grid.Outbox = m.spec(leftW, outboxH)
	grid.Composer = m.spec(leftW, composerHeight)
	grid.SidePanel = m.spec(rightW, 0)

	// Side panel tabs render their own panels directly.
	// Reserve 1 row for the side tab bar, so right column height matches left.
	sideContentH := mainH - 1
	if sideContentH < 1 {
		sideContentH = 1
	}
	feedH := int(math.Round(float64(sideContentH) * 0.40))
	if feedH < 7 {
		feedH = 7
	}
	if feedH > sideContentH-7 {
		feedH = max(1, sideContentH-7)
	}
	detailH := sideContentH - feedH
	if detailH < 1 {
		detailH = 1
	}
	grid.ActivityFeed = m.spec(rightW, feedH)
	grid.ActivityDetail = m.spec(rightW, detailH)
	grid.Plan = m.spec(rightW, sideContentH)

	// Tasks tab: Current task + Queue + Stats.
	currentTaskH := 7
	statsH := 5
	if sideContentH < 14 {
		// Tight terminals: prioritize queue.
		currentTaskH = 5
		statsH = 4
	}
	queueH := sideContentH - currentTaskH - statsH
	if queueH < 1 {
		queueH = 1
		statsH = max(1, sideContentH-currentTaskH-queueH)
	}
	grid.CurrentTask = m.spec(rightW, currentTaskH)
	grid.TaskQueue = m.spec(rightW, queueH)
	grid.Stats = m.spec(rightW, statsH)
	grid.Memory = m.spec(leftW, 0)
	return grid
}

func (m *Manager) spec(width, height int) PanelSpec {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	contentW := max(0, width-m.frameW)
	contentH := max(0, height-m.frameH-m.titleHeight)
	return PanelSpec{
		Width:         width,
		Height:        height,
		ContentWidth:  contentW,
		ContentHeight: contentH,
		FrameWidth:    m.frameW,
		FrameHeight:   m.frameH,
		TitleHeight:   m.titleHeight,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
