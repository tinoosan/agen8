package tui

func (m *monitorModel) closeAllPickers() {
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
	if m.newSessionWizardOpen {
		m.closeNewSessionWizard()
	}
}
