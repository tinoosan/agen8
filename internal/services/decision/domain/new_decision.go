package domain

import (
	"strings"
	"time"

	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type NewLogInput struct {
	ProjectID              string
	SpaceID                string
	RunID                  string
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
	PlanRef                string
}

func NewLog(input NewLogInput, now time.Time) (Decision, error) {
	decision := Decision{
		ProjectID:      strings.TrimSpace(input.ProjectID),
		SpaceID:        strings.TrimSpace(input.SpaceID),
		Source:         DecisionSourceAgent,
		SourceIdentity: strings.TrimSpace(input.MemberID),
		RunID:          strings.TrimSpace(input.RunID),
		Title:          strings.TrimSpace(input.Title),
		Confidence:     input.Confidence,
		TaskRef:        strings.TrimSpace(input.TaskRef),
		KeyResultRef:   strings.TrimSpace(input.KeyResultRef),
		MissionRef:     strings.TrimSpace(input.MissionRef),
		PlanRef:        strings.TrimSpace(input.PlanRef),
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

type NewAskUserInput struct {
	ProjectID    string
	SpaceID      string
	RunID        string
	MemberID     string
	Title        string
	Context      string
	Questions    []humaninput.Question
	Answers      []humaninput.Answer
	Cancelled    bool
	TaskRef      string
	KeyResultRef string
	MissionRef   string
	PlanRef      string
}

func NewAskUser(input NewAskUserInput, now time.Time) (Decision, error) {
	decision := Decision{
		ProjectID:      strings.TrimSpace(input.ProjectID),
		SpaceID:        strings.TrimSpace(input.SpaceID),
		Source:         DecisionSourceAgent,
		SourceIdentity: strings.TrimSpace(input.MemberID),
		RunID:          strings.TrimSpace(input.RunID),
		Title:          strings.TrimSpace(input.Title),
		TaskRef:        strings.TrimSpace(input.TaskRef),
		KeyResultRef:   strings.TrimSpace(input.KeyResultRef),
		MissionRef:     strings.TrimSpace(input.MissionRef),
		PlanRef:        strings.TrimSpace(input.PlanRef),
		CreatedAt:      now.UTC(),
		AskUser: &AskUserPayload{
			Context:   strings.TrimSpace(input.Context),
			Questions: append([]humaninput.Question(nil), input.Questions...),
			Answers:   append([]humaninput.Answer(nil), input.Answers...),
			Cancelled: input.Cancelled,
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
