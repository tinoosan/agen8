package decision

import (
	"encoding/json"
	"fmt"
	"strings"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
)

type decisionEntry struct {
	ID                     string   `json:"id"`
	Kind                   string   `json:"kind"`
	Title                  string   `json:"title"`
	InvalidationConditions []string `json:"invalidationConditions,omitempty"`
	TaskRef                string   `json:"taskRef,omitempty"`
	KeyResultRef           string   `json:"keyResultRef,omitempty"`
	MissionRef             string   `json:"missionRef,omitempty"`
	MemberID               string   `json:"memberId,omitempty"`
	MemberName             string   `json:"memberName,omitempty"`
	SourceType             string   `json:"sourceType,omitempty"`
}

func resultEntry(result decisionapp.Result) decisionEntry {
	return decisionEntry{
		ID:                     strings.TrimSpace(result.ID),
		Kind:                   strings.TrimSpace(result.Kind),
		Title:                  strings.TrimSpace(result.Title),
		InvalidationConditions: append([]string(nil), result.InvalidationConditions...),
		TaskRef:                strings.TrimSpace(result.TaskRef),
		KeyResultRef:           strings.TrimSpace(result.KeyResultRef),
		MissionRef:             strings.TrimSpace(result.MissionRef),
		MemberID:               strings.TrimSpace(result.MemberID),
		MemberName:             strings.TrimSpace(result.MemberName),
		SourceType:             strings.TrimSpace(result.SourceType),
	}
}

func domainEntry(decision decisiondomain.Decision) decisionEntry {
	entry := decisionEntry{
		ID:           strings.TrimSpace(string(decision.ID)),
		Kind:         strings.TrimSpace(string(decision.Kind())),
		Title:        strings.TrimSpace(decision.Title),
		TaskRef:      strings.TrimSpace(decision.TaskRef),
		KeyResultRef: strings.TrimSpace(decision.KeyResultRef),
		MissionRef:   strings.TrimSpace(decision.MissionRef),
		MemberID:     strings.TrimSpace(decision.SourceIdentity),
		SourceType:   strings.TrimSpace(string(decision.Source)),
	}
	if decision.Log != nil {
		entry.InvalidationConditions = append([]string(nil), decision.Log.InvalidationConditions...)
	}
	return entry
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
		return "", fmt.Errorf("decision: encode structured response: %w", err)
	}
	return string(encoded), nil
}
