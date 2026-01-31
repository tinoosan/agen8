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

// Calculate produces a GridLayout for the given screen size. inputHeight is the
// total height of the composer bar (help text + textarea) supplied by the
// caller.
func (m *Manager) Calculate(width, height, inputHeight int) GridLayout {
	const (
		headerHeight   = 1
		outboxHeight   = 6
		memoryHeight   = 6
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

	mainH := height - headerHeight - outboxHeight - memoryHeight - inputHeight
	if mainH < minMainHeight {
		mainH = minMainHeight
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

	activityH := max(minPanelHeight, int(math.Round(float64(mainH)*0.45)))
	outputH := mainH - activityH
	if outputH < minPanelHeight {
		outputH = minPanelHeight
		activityH = max(minPanelHeight, mainH-outputH)
	}

	detailH := max(minDetailH, int(math.Round(float64(mainH)*0.40)))
	// Ensure room for the remaining four right-hand panels.
	if detailH > mainH-(minPanelHeight*4) {
		detailH = max(minPanelHeight, mainH-(minPanelHeight*4))
	}

	remaining := max(0, mainH-detailH)
	taskH, planH, queueH, statsH := allocateRemaining(remaining, minPanelHeight)

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
