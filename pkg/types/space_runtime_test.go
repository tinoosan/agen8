package types

import (
	"testing"
	"time"
)

func TestSpaceDisplayNamePrefersNameThenTitleThenGoal(t *testing.T) {
	createdAt := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2 * time.Hour)
	space := SpaceRuntime{
		SpaceID:       "space-1",
		Name:          "Platform",
		Title:         "Platform",
		CurrentGoal:   "Ship DM-09 slice",
		Plan:          "1. Add SpaceRuntime\n2. Use helper",
		Summary:       "Mapping is explicit",
		ProjectID:     "project-1",
		ApprovalsMode: "enabled",
		System:        true,
		CreatedAt:     &createdAt,
		UpdatedAt:     &updatedAt,
		TokenUsage: TokenUsage{
			InputTokens:  11,
			OutputTokens: 7,
			TotalTokens:  18,
			CostUSD:      0.42,
		},
	}
	wantUsage := space.TokenUsage

	if space.SpaceID != "space-1" {
		t.Fatalf("SpaceID=%q", space.SpaceID)
	}
	if space.DisplayName() != "Platform" {
		t.Fatalf("DisplayName=%q", space.DisplayName())
	}
	if space.CurrentGoal != "Ship DM-09 slice" || space.Plan != "1. Add SpaceRuntime\n2. Use helper" || space.Summary != "Mapping is explicit" {
		t.Fatalf("unexpected space context: %+v", space)
	}
	if space.ProjectID != "project-1" || space.ApprovalsMode != "enabled" {
		t.Fatalf("unexpected space metadata: %+v", space)
	}
	if !space.System {
		t.Fatalf("System=false want true")
	}
	if space.CreatedAt != &createdAt || space.UpdatedAt != &updatedAt {
		t.Fatalf("timestamps not preserved")
	}
	if space.TokenUsage != wantUsage {
		t.Fatalf("TokenUsage=%+v want %+v", space.TokenUsage, wantUsage)
	}
}

func TestSpaceDisplayNameFallsBackToGoal(t *testing.T) {
	space := SpaceRuntime{SpaceID: "space-2", CurrentGoal: "Unblock routing"}
	if space.DisplayName() != "Unblock routing" {
		t.Fatalf("DisplayName=%q", space.DisplayName())
	}
}
