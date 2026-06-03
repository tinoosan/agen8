package operator

import (
	"encoding/json"
	"fmt"
	"strings"

	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

type operatorActionEntry struct {
	ID                   string `json:"id"`
	Kind                 string `json:"kind"`
	Status               string `json:"status"`
	Title                string `json:"title"`
	TaskRef              string `json:"taskRef,omitempty"`
	KeyResultRef         string `json:"keyResultRef,omitempty"`
	MissionRef           string `json:"missionRef,omitempty"`
	Blocking             bool   `json:"blocking"`
	RequiresVerification bool   `json:"requiresVerification"`
	CreatedAt            string `json:"createdAt"`
}

type escalationEntryView struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Status         string  `json:"status"`
	Title          string  `json:"title"`
	TaskRef        string  `json:"taskRef,omitempty"`
	KeyResultRef   string  `json:"keyResultRef,omitempty"`
	MissionRef     string  `json:"missionRef,omitempty"`
	Recommendation string  `json:"recommendation,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	CreatedAt      string  `json:"createdAt"`
}

func actionEntry(action operatordomain.OperatorAction) operatorActionEntry {
	return operatorActionEntry{
		ID:                   strings.TrimSpace(string(action.ID)),
		Kind:                 "operator_action",
		Status:               strings.TrimSpace(string(action.Status)),
		Title:                strings.TrimSpace(action.Title),
		TaskRef:              strings.TrimSpace(action.TaskRef),
		KeyResultRef:         strings.TrimSpace(action.KeyResultRef),
		MissionRef:           strings.TrimSpace(action.MissionRef),
		Blocking:             action.Blocking,
		RequiresVerification: action.RequiresVerification,
		CreatedAt:            action.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func escalationEntry(escalation operatordomain.Escalation) escalationEntryView {
	return escalationEntryView{
		ID:             strings.TrimSpace(string(escalation.ID)),
		Kind:           "escalation",
		Status:         strings.TrimSpace(string(escalation.Status)),
		Title:          strings.TrimSpace(escalation.Title),
		TaskRef:        strings.TrimSpace(escalation.TaskRef),
		KeyResultRef:   strings.TrimSpace(escalation.KeyResultRef),
		MissionRef:     strings.TrimSpace(escalation.MissionRef),
		Recommendation: strings.TrimSpace(escalation.Recommendation),
		Confidence:     escalation.Confidence,
		CreatedAt:      escalation.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func resultFromStructured(structured map[string]any) (Result, error) {
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("operator: encode structured response: %w", err)
	}
	return string(encoded), nil
}
