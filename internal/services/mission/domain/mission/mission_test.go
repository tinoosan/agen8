package mission

import (
	"testing"
	"time"
)

func TestNewMissionCreatesDraftMission(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	got, err := NewMission(NewMissionInput{
		ID:        MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	if got.Status != MissionStatusDraft {
		t.Fatalf("status=%q want %q", got.Status, MissionStatusDraft)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps not set from input now")
	}
}

func TestMissionUpdateDetails(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(time.Hour)
	mission, err := NewMission(NewMissionInput{
		ID:        MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	got, err := mission.UpdateDetails("Updated launch", "New scope", updatedAt)
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if got.Title != "Updated launch" {
		t.Fatalf("Title=%q", got.Title)
	}
	if got.Description != "New scope" {
		t.Fatalf("Description=%q", got.Description)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("UpdatedAt=%v want %v", got.UpdatedAt, updatedAt)
	}
}

func TestMissionLifecycleTransitions(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	mission, err := NewMission(NewMissionInput{
		ID:        MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	active, err := mission.Activate(now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if active.Status != MissionStatusActive {
		t.Fatalf("active status=%q", active.Status)
	}
	paused, err := active.Pause(now.Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Status != MissionStatusPaused || paused.PausedAt == nil {
		t.Fatalf("paused=%+v", paused)
	}
	activeAgain, err := paused.Activate(now.Add(3 * time.Hour))
	if err != nil {
		t.Fatalf("Activate paused: %v", err)
	}
	if activeAgain.Status != MissionStatusActive || activeAgain.PausedAt != nil {
		t.Fatalf("activeAgain=%+v", activeAgain)
	}
	completed, err := activeAgain.Complete(now.Add(4 * time.Hour))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if completed.Status != MissionStatusCompleted || completed.CompletedAt == nil {
		t.Fatalf("completed=%+v", completed)
	}
	reactivated, err := completed.Activate(now.Add(5 * time.Hour))
	if err != nil {
		t.Fatalf("Activate completed: %v", err)
	}
	if reactivated.Status != MissionStatusActive || reactivated.CompletedAt != nil {
		t.Fatalf("reactivated=%+v", reactivated)
	}
	archived, err := reactivated.Archive(now.Add(6 * time.Hour))
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if archived.Status != MissionStatusArchived {
		t.Fatalf("archived status=%q", archived.Status)
	}
}

func TestMissionRejectsInvalidTransitions(t *testing.T) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	mission, err := NewMission(NewMissionInput{
		ID:        MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	if _, err := mission.Pause(now); err == nil {
		t.Fatal("Pause draft error is nil")
	}
	if _, err := mission.Complete(now); err == nil {
		t.Fatal("Complete draft error is nil")
	}
	archived, err := mission.Archive(now)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := archived.Activate(now); err == nil {
		t.Fatal("Activate archived error is nil")
	}
	if _, err := archived.UpdateDetails("Title", "", now); err == nil {
		t.Fatal("UpdateDetails archived error is nil")
	}
}
