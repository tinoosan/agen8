package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type newSessionWizardItem struct {
	mode      string
	title     string
	desc      string
	sessionID string
	runID     string
}

func (i newSessionWizardItem) Title() string       { return i.title }
func (i newSessionWizardItem) Description() string { return i.desc }
func (i newSessionWizardItem) FilterValue() string {
	return strings.TrimSpace(i.mode + " " + i.title + " " + i.desc + " " + i.sessionID)
}

func (m *monitorModel) openNewSessionWizard() tea.Cmd {
	m.helpModalOpen = false
	if m.sessionPickerOpen {
		m.closeSessionPicker()
	}
	if m.agentPickerOpen {
		m.closeAgentPicker()
	}
	if m.profilePickerOpen {
		m.closeProfilePicker()
	}
	if m.teamPickerOpen {
		m.closeTeamPicker()
	}
	if m.modelPickerOpen {
		m.closeModelPicker()
	}
	if m.reasoningEffortPickerOpen {
		m.closeReasoningEffortPicker()
	}
	if m.reasoningSummaryPickerOpen {
		m.closeReasoningSummaryPicker()
	}
	if m.filePickerOpen {
		m.closeFilePicker()
	}

	var items []list.Item
	if m.session != nil {
		if sessions, err := m.session.ListSessionsPaginated(m.ctx, pkgstore.SessionFilter{
			Limit:    5,
			SortBy:   "updated_at",
			SortDesc: true,
		}); err == nil {
			for _, s := range sessions {
				title := strings.TrimSpace(s.Title)
				if title == "" {
					title = "Untitled Session"
				}
				lastActive := "unknown"
				if !s.UpdatedAt.IsZero() {
					lastActive = timeutil.Since(timeutil.OrNow(s.UpdatedAt)).Round(time.Second).String()
				}
				activeFor := "unknown"
				if !s.CreatedAt.IsZero() {
					activeFor = timeutil.Since(timeutil.OrNow(s.CreatedAt)).Round(time.Second).String()
				}
				items = append(items, newSessionWizardItem{
					mode:      "resume",
					title:     "Resume: " + truncateText(title, 40),
					desc:      "Last active: " + lastActive + " · Active for: " + activeFor,
					sessionID: s.SessionID,
				})
			}
		}
	}

	items = append(items,
		newSessionWizardItem{mode: "standalone", title: "New Standalone Session", desc: "single agent; choose profile and start"},
		newSessionWizardItem{mode: "team", title: "New Team Session", desc: "multi-role team; profile is immutable"},
	)

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "New Session Wizard"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Bold(true)
	l.Select(0)
	m.newSessionWizardList = l
	m.newSessionWizardOpen = true
	return nil
}

func (m *monitorModel) closeNewSessionWizard() {
	m.newSessionWizardOpen = false
	m.newSessionWizardList = list.Model{}
}

func (m *monitorModel) updateNewSessionWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.closeNewSessionWizard()
		return m, nil
	case tea.KeyEnter:
		item, ok := m.newSessionWizardList.SelectedItem().(newSessionWizardItem)
		if !ok {
			return m, nil
		}
		m.closeNewSessionWizard()

		if strings.EqualFold(strings.TrimSpace(item.mode), "resume") {
			targetSessionID := strings.TrimSpace(item.sessionID)
			if targetSessionID == "" {
				return m, nil
			}
			// Use the session picker logic to find the best run to resume
			// We can trigger the picker logic manually or replicate it.
			// Replicating for now to keep wizard self-contained.
			return m, func() tea.Msg {
				sess, err := m.session.LoadSession(m.ctx, targetSessionID)
				if err != nil {
					return commandLinesMsg{lines: []string{"[error] failed to load session: " + err.Error()}}
				}
				if strings.TrimSpace(sess.TeamID) != "" {
					return monitorSwitchTeamMsg{TeamID: strings.TrimSpace(sess.TeamID)}
				}

				// Find best run
				var agents protocol.AgentListResult
				if err := m.rpcRoundTrip(protocol.MethodAgentList, protocol.AgentListParams{
					ThreadID:  protocol.ThreadID(targetSessionID),
					SessionID: targetSessionID,
				}, &agents); err != nil {
					return commandLinesMsg{lines: []string{"[error] failed to list agents: " + err.Error()}}
				}

				var bestRunID string
				var bestScore int

				for _, ag := range agents.Agents {
					candidate := strings.TrimSpace(ag.RunID)
					if candidate == "" {
						continue
					}
					isTopLevel := strings.TrimSpace(ag.ParentRunID) == ""
					isRunning := strings.EqualFold(strings.TrimSpace(ag.Status), types.RunStatusRunning)

					score := 0
					if isTopLevel && isRunning {
						score = 3
					} else if isTopLevel {
						score = 2
					} else if isRunning {
						score = 1
					}

					if bestRunID == "" || score > bestScore {
						bestRunID = candidate
						bestScore = score
					}
				}

				if bestRunID != "" {
					return monitorSwitchRunMsg{RunID: bestRunID}
				}
				return commandLinesMsg{lines: []string{"[error] no valid runs definitions found in session"}}
			}
		}

		if strings.EqualFold(strings.TrimSpace(item.mode), "team") {
			return m, m.openProfilePickerFor("new-team", true)
		}
		return m, m.openProfilePickerFor("new-standalone", false)
	default:
		var cmd tea.Cmd
		m.newSessionWizardList, cmd = m.newSessionWizardList.Update(msg)
		return m, cmd
	}
}

func (m *monitorModel) renderNewSessionWizard(base string) string {
	maxModalW := max(1, m.width-8)
	modalWidth := min(84, maxModalW)
	minModalW := min(56, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}
	maxModalH := max(1, m.height-8)
	modalHeight := min(18, maxModalH)
	minModalH := min(10, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}
	listHeight := modalHeight - 4
	if listHeight < 4 {
		listHeight = 4
	}
	m.newSessionWizardList.SetWidth(modalWidth - 4)
	m.newSessionWizardList.SetHeight(listHeight)

	content := m.newSessionWizardList.View() + "\n" + kit.StyleDim.Render("Enter: next • Esc: close")
	opts := kit.ModalOptions{
		Content:      content,
		ScreenWidth:  m.width,
		ScreenHeight: m.height,
		Width:        modalWidth,
		Height:       modalHeight,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}
	_ = base
	return kit.RenderOverlay(opts)
}
