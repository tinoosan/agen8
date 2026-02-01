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
	const headerHeight = 1
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
	// Zero-height specs for panels not shown in compact mode
	grid.ActivityFeed = m.spec(width, 0)
	grid.ActivityDetail = m.spec(width, 0)
	grid.CurrentTask = m.spec(width, 0)
	grid.Plan = m.spec(width, 0)
	grid.TaskQueue = m.spec(width, 0)
	grid.Stats = m.spec(width, 0)
	grid.Outbox = m.spec(width, 0)
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
// Reserves header, status bar, composer height, and gaps; allocates remainder to
// left (AgentOutput flex + Outbox fixed) and right (Activity, ActivityDetail,
// CurrentTask, Queue, Plan, Stats). Outbox and Composer use left column width only.
func (m *Manager) CalculateDashboard(width, height, composerHeight, outboxHeight int) GridLayout {
	const (
		minLeftWidth  = 60
		minRightWidth = 32
		gapCols       = 1
		outboxFixedH  = 6
		outboxMinH    = 4
		activityH     = 8
		currentTaskH  = 4
		queueH        = 5
		planH         = 8
		statsH        = 5
		minDetailH    = 5
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
	reserved := DashHeaderHeight + DashStatusBarHeight + DashWarningHeight + composerHeight + DashGapAfterHeader + DashGapBeforeComposer
	remainingHeight := height - reserved
	if remainingHeight < 1 {
		remainingHeight = 1
	}

	leftW := int(math.Round(float64(width) * 0.66))
	if leftW < minLeftWidth {
		leftW = minLeftWidth
	}
	if leftW > width-minRightWidth-gapCols {
		leftW = max(0, width-minRightWidth-gapCols)
	}
	rightW := width - leftW - gapCols
	if rightW < minRightWidth {
		rightW = minRightWidth
		leftW = max(0, width-rightW-gapCols)
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

	fixedRight := activityH + currentTaskH + queueH + planH + statsH
	detailH := mainH - fixedRight
	if mainH < fixedRight+minDetailH {
		detailH = 1
		rest := mainH - 1
		activityH := rest * 8 / 30
		taskH := rest * 4 / 30
		queueH := rest * 5 / 30
		planH := rest * 8 / 30
		statsH := rest * 5 / 30
		grid := GridLayout{ScreenWidth: width, ScreenHeight: height}
		grid.AgentOutput = m.spec(leftW, agentOutputH)
		grid.Outbox = m.spec(leftW, outboxH)
		grid.Composer = m.spec(leftW, composerHeight)
		grid.ActivityFeed = m.spec(rightW, activityH)
		grid.ActivityDetail = m.spec(rightW, detailH)
		grid.CurrentTask = m.spec(rightW, taskH)
		grid.TaskQueue = m.spec(rightW, queueH)
		grid.Plan = m.spec(rightW, planH)
		grid.Stats = m.spec(rightW, statsH)
		grid.Memory = m.spec(leftW, 0)
		return grid
	}
	if detailH < 1 {
		detailH = 1
	}

	grid := GridLayout{ScreenWidth: width, ScreenHeight: height}
	grid.AgentOutput = m.spec(leftW, agentOutputH)
	grid.Outbox = m.spec(leftW, outboxH)
	grid.Composer = m.spec(leftW, composerHeight)
	grid.ActivityFeed = m.spec(rightW, activityH)
	grid.ActivityDetail = m.spec(rightW, detailH)
	grid.CurrentTask = m.spec(rightW, currentTaskH)
	grid.TaskQueue = m.spec(rightW, queueH)
	grid.Plan = m.spec(rightW, planH)
	grid.Stats = m.spec(rightW, statsH)
	grid.Memory = m.spec(leftW, 0)
	return grid
}

// Calculate produces a GridLayout for the given screen size. inputHeight is the
// total height of the composer bar (help text + textarea) supplied by the
// caller. outboxHeight and memoryHeight allow callers to dynamically size or
// hide those panels (use 0 to hide).
func (m *Manager) Calculate(width, height, inputHeight, outboxHeight, memoryHeight int) GridLayout {
	const (
		headerHeight   = 1
		minMainHeight  = 8
		minLeftWidth   = 36
		minRightWidth  = 32
		minPanelHeight = 4
		minDetailH     = 8
	)

	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	if inputHeight < 0 {
		inputHeight = 0
	}
	if outboxHeight < 0 {
		outboxHeight = 0
	}
	if memoryHeight < 0 {
		memoryHeight = 0
	}

	mainH := height - headerHeight - outboxHeight - memoryHeight - inputHeight
	// CRITICAL FIX: Do NOT enforce minMainHeight if it pushes total height > screen height.
	// We allow mainH to shrink (even to 0) rather than creating a layout that is taller
	// than the terminal, which causes the top to be clipped.
	if mainH < 1 {
		mainH = 1
	}

	leftW := int(math.Round(float64(width) * 0.60))
	if leftW < minLeftWidth {
		leftW = minLeftWidth
	}
	if leftW > width-minRightWidth {
		leftW = max(0, width-minRightWidth)
	}
	rightW := width - leftW
	if rightW < minRightWidth {
		rightW = minRightWidth
		leftW = max(0, width-rightW)
	}

	// Proportional split for Activity vs Output.
	// We want roughly 45% for activity, but MUST respect minPanelHeight IF we have space.
	// If space is tight (mainH < 8), we simply split what we have.
	var activityH, outputH int
	if mainH < minPanelHeight*2 {
		// Not enough room for minimums. Split proportionally to avoid overflow.
		activityH = mainH / 2
		outputH = mainH - activityH
	} else {
		// Normal sizing with minimum guarantees.
		activityH = int(math.Round(float64(mainH) * 0.45))
		if activityH < minPanelHeight {
			activityH = minPanelHeight
		}
		outputH = mainH - activityH
		if outputH < minPanelHeight {
			outputH = minPanelHeight
			activityH = mainH - outputH
		}
	}

	var detailH, taskH, planH, queueH, statsH int
	if mainH < 24 { // Adjusted threshold: 8(detail) + 4*4(others) = 24.
		// Height is constrained. Split proportionally.
		// Detail gets ~30%, others ~17.5% each.
		detailH = int(float64(mainH) * 0.30)
		if detailH < 1 {
			detailH = 1
		}
		rem := mainH - detailH
		// Split remainder evenly among 4.
		base := rem / 4
		taskH, planH, queueH, statsH = base, base, base, base
		// Assign remainder to Stats to ensure sum = mainH
		statsH = rem - (base * 3)
	} else {
		detailH = max(minDetailH, int(math.Round(float64(mainH)*0.40)))
		// Ensure room for the remaining four right-hand panels.
		if detailH > mainH-(minPanelHeight*4) {
			detailH = max(minPanelHeight, mainH-(minPanelHeight*4))
		}
		remaining := max(0, mainH-detailH)
		taskH, planH, queueH, statsH = allocateRemaining(remaining, minPanelHeight)
	}

	// Clamp totals so right column height matches mainH.
	rightTotal := detailH + taskH + planH + queueH + statsH
	if diff := mainH - rightTotal; diff != 0 {
		statsH += diff
	}
	if statsH < minPanelHeight {
		deficit := minPanelHeight - statsH
		statsH = minPanelHeight
		// Borrow evenly from task, plan, queue while keeping their minimums.
		for deficit > 0 {
			adjusted := false
			for _, h := range []*int{&taskH, &planH, &queueH} {
				if *h > minPanelHeight {
					*h--
					deficit--
					adjusted = true
					if deficit == 0 {
						break
					}
				}
			}
			if !adjusted {
				break
			}
		}
	}
	// Final normalize so totals match mainH.
	rightTotal = detailH + taskH + planH + queueH + statsH
	if diff := mainH - rightTotal; diff != 0 {
		statsH = max(minPanelHeight, statsH+diff)
	}

	grid := GridLayout{ScreenWidth: width, ScreenHeight: height}

	grid.ActivityFeed = m.spec(leftW, activityH)
	grid.AgentOutput = m.spec(leftW, mainH-activityH)
	grid.ActivityDetail = m.spec(rightW, detailH)
	grid.CurrentTask = m.spec(rightW, taskH)
	grid.Plan = m.spec(rightW, planH)
	grid.TaskQueue = m.spec(rightW, queueH)
	grid.Stats = m.spec(rightW, statsH)

	grid.Outbox = m.spec(width, outboxHeight)
	grid.Memory = m.spec(width, memoryHeight)
	grid.Composer = m.spec(width, inputHeight)

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

func allocateRemaining(total, minPanel int) (task, plan, queue, stats int) {
	if total <= 0 {
		return minPanel, minPanel, minPanel, minPanel
	}
	base := total / 4
	if base < minPanel {
		base = minPanel
	}
	task, plan, queue = base, base, base
	stats = total - (task + plan + queue)
	if stats < minPanel {
		stats = minPanel
		need := task + plan + queue + stats - total
		for need > 0 {
			if task > minPanel {
				task--
				need--
				continue
			}
			if plan > minPanel {
				plan--
				need--
				continue
			}
			if queue > minPanel {
				queue--
				need--
				continue
			}
			break
		}
	}
	return
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
