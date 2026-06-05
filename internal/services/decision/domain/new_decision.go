package domain

import (
	"strings"
	"time"
)

type NewLogInput struct {
	ProjectID              string
	MemberID               string
	Title                  string
	Rationale              string
	Context                string
	AlternativesRejected   string
	InvalidationConditions []string
	Confidence             float64
	Outcome                string
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
}

func NewLog(input NewLogInput, now time.Time) (Decision, error) {
	decision := Decision{
		ProjectID:      strings.TrimSpace(input.ProjectID),
		Source:         DecisionSourceAgent,
		SourceIdentity: strings.TrimSpace(input.MemberID),
		Title:          strings.TrimSpace(input.Title),
		Confidence:     input.Confidence,
		TaskRef:        strings.TrimSpace(input.TaskRef),
		KeyResultRef:   strings.TrimSpace(input.KeyResultRef),
		MissionRef:     strings.TrimSpace(input.MissionRef),
		CreatedAt:      now.UTC(),
		Log: &LogPayload{
			Rationale:              strings.TrimSpace(input.Rationale),
			Context:                strings.TrimSpace(input.Context),
			AlternativesRejected:   strings.TrimSpace(input.AlternativesRejected),
			InvalidationConditions: normalizeStringSlice(input.InvalidationConditions),
			Outcome:                strings.TrimSpace(input.Outcome),
		},
	}
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func normalizeStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
