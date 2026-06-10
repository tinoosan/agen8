package mission

import (
	"encoding/json"
	"fmt"
	"time"

	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type missionEntry struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	PausedAt    string `json:"pausedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type keyResultEntry struct {
	ID                    string   `json:"id"`
	MissionID             string   `json:"missionId,omitempty"`
	Title                 string   `json:"title,omitempty"`
	Description           string   `json:"description,omitempty"`
	MeasurementType       string   `json:"measurementType,omitempty"`
	Direction             string   `json:"direction,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	Baseline              *float64 `json:"baseline,omitempty"`
	TargetValue           float64  `json:"targetValue,omitempty"`
	CurrentValue          float64  `json:"currentValue,omitempty"`
	ProgressPercent       int      `json:"progressPercent"`
	Status                string   `json:"status,omitempty"`
	LastMilestoneNotified int      `json:"lastMilestoneNotified,omitempty"`
	Version               int64    `json:"version,omitempty"`
}

// leanMissionEntry / leanKeyResultEntry are the mutation + list-row shapes. They
// drop the description and timestamps the caller already supplied; the KR keeps
// version so a follow-up kr_progress can pass expected_version.
type leanMissionEntry struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

type leanKeyResultEntry struct {
	ID              string `json:"id"`
	MissionID       string `json:"missionId,omitempty"`
	Title           string `json:"title,omitempty"`
	Status          string `json:"status,omitempty"`
	ProgressPercent int    `json:"progressPercent"`
	Version         int64  `json:"version,omitempty"`
}

type progressEntry struct {
	ID              string  `json:"id"`
	KeyResultID     string  `json:"keyResultId"`
	PreviousValue   float64 `json:"previousValue"`
	NewValue        float64 `json:"newValue"`
	ProgressPercent int     `json:"progressPercent"`
	UpdatedBy       string  `json:"updatedBy,omitempty"`
	Note            string  `json:"note,omitempty"`
	CreatedAt       string  `json:"createdAt,omitempty"`
}

type progressKeyResultEntry struct {
	ID              string `json:"id"`
	Title           string `json:"title,omitempty"`
	Status          string `json:"status"`
	ProgressPercent int    `json:"progressPercent"`
}

type lifecycleHistoryEntry struct {
	EventID         string `json:"eventId,omitempty"`
	MissionID       string `json:"missionId"`
	KeyResultID     string `json:"keyResultId,omitempty"`
	Type            string `json:"type"`
	Action          string `json:"action"`
	Status          string `json:"status,omitempty"`
	Note            string `json:"note,omitempty"`
	Actor           string `json:"actor,omitempty"`
	Origin          string `json:"origin,omitempty"`
	Message         string `json:"message,omitempty"`
	ProgressPercent string `json:"progressPercent,omitempty"`
	Timestamp       string `json:"timestamp,omitempty"`
}

// missionResult / keyResultResult return the FULL entry and are used only by the
// detail fetches (get / kr_get). Mutations and list rows use the lean variants
// below so the model isn't re-sent the description/timestamps it already holds.
func missionResult(action string, mission missiondomain.Mission) (Result, error) {
	return resultFromStructured(map[string]any{
		"ok":      true,
		"tool":    Name,
		"action":  action,
		"mission": toMissionEntry(mission),
	})
}

func keyResultResult(action string, keyResult krdomain.KeyResult) (Result, error) {
	return resultFromStructured(map[string]any{
		"ok":        true,
		"tool":      Name,
		"action":    action,
		"keyResult": toKeyResultEntry(keyResult),
	})
}

// leanMissionResult is the mutation response (create/update/activate/pause/
// complete/archive): id, title, status, plus an optional lifecycle note.
func leanMissionResult(action string, mission missiondomain.Mission, note string) (Result, error) {
	structured := map[string]any{
		"ok":      true,
		"tool":    Name,
		"action":  action,
		"mission": toLeanMissionEntry(mission),
	}
	addNote(structured, note)
	return resultFromStructured(structured)
}

// leanKeyResultResult is the mutation response (kr_create/update/progress/drop/
// reopen). It keeps version so a follow-up kr_progress can pass expected_version.
func leanKeyResultResult(action string, keyResult krdomain.KeyResult, note string) (Result, error) {
	structured := map[string]any{
		"ok":        true,
		"tool":      Name,
		"action":    action,
		"keyResult": toLeanKeyResultEntry(keyResult),
	}
	addNote(structured, note)
	return resultFromStructured(structured)
}

func addNote(structured map[string]any, note string) {
	if note != "" {
		structured["note"] = note
	}
}

func missionListResult(missions []missiondomain.Mission, input requestInput) (Result, error) {
	rows := make([]leanMissionEntry, 0, len(missions))
	for _, mission := range missions {
		rows = append(rows, toLeanMissionEntry(mission))
	}
	structured := map[string]any{
		"ok":       true,
		"tool":     Name,
		"action":   "list",
		"missions": rows,
		"count":    len(rows),
		"limit":    input.Limit,
		"offset":   input.Offset,
	}
	return resultFromStructured(structured)
}

func keyResultListResult(keyResults []krdomain.KeyResult, input requestInput) (Result, error) {
	rows := make([]leanKeyResultEntry, 0, len(keyResults))
	for _, keyResult := range keyResults {
		rows = append(rows, toLeanKeyResultEntry(keyResult))
	}
	structured := map[string]any{
		"ok":         true,
		"tool":       Name,
		"action":     "kr_list",
		"keyResults": rows,
		"count":      len(rows),
		"limit":      input.Limit,
		"offset":     input.Offset,
	}
	return resultFromStructured(structured)
}

func progressResult(progress missionapp.MissionProgress) (Result, error) {
	statusCounts := make(map[string]int, len(progress.StatusCounts))
	for status, count := range progress.StatusCounts {
		statusCounts[string(status)] = count
	}
	blocking := make([]progressKeyResultEntry, 0, len(progress.BlockingKeyResults))
	for _, keyResult := range progress.BlockingKeyResults {
		blocking = append(blocking, progressKeyResultEntry{
			ID:              string(keyResult.ID),
			Title:           keyResult.Title,
			Status:          string(keyResult.Status),
			ProgressPercent: keyResult.ProgressPercent,
		})
	}
	structured := map[string]any{
		"ok":                 true,
		"tool":               Name,
		"action":             "progress",
		"missionId":          string(progress.MissionID),
		"progressPercent":    progress.ProgressPercent,
		"keyResultCount":     progress.KeyResultCount,
		"statusCounts":       statusCounts,
		"blockingKeyResults": blocking,
	}
	return resultFromStructured(structured)
}

func historyResult(entries []krdomain.ProgressEntry) (Result, error) {
	rows := make([]progressEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, progressEntry{
			ID:              entry.ID,
			KeyResultID:     string(entry.KeyResultID),
			PreviousValue:   entry.PreviousValue,
			NewValue:        entry.NewValue,
			ProgressPercent: entry.ProgressPercent,
			UpdatedBy:       entry.UpdatedBy,
			Note:            entry.Note,
			CreatedAt:       formatTime(entry.CreatedAt),
		})
	}
	structured := map[string]any{
		"ok":      true,
		"tool":    Name,
		"action":  "kr_history",
		"entries": rows,
		"count":   len(rows),
	}
	return resultFromStructured(structured)
}

func lifecycleHistoryResult(history missionapp.LifecycleHistory) (Result, error) {
	rows := make([]lifecycleHistoryEntry, 0, len(history.Entries))
	for _, entry := range history.Entries {
		rows = append(rows, lifecycleHistoryEntry{
			EventID:         entry.EventID,
			MissionID:       string(entry.MissionID),
			KeyResultID:     string(entry.KeyResultID),
			Type:            entry.Type,
			Action:          entry.Action,
			Status:          entry.Status,
			Note:            entry.Note,
			Actor:           entry.Actor,
			Origin:          entry.Origin,
			Message:         entry.Message,
			ProgressPercent: entry.ProgressPercent,
			Timestamp:       formatTime(entry.Timestamp),
		})
	}
	structured := map[string]any{
		"ok":        true,
		"tool":      Name,
		"action":    "history",
		"missionId": string(history.MissionID),
		"entries":   rows,
		"count":     history.Count,
		"limit":     history.Limit,
		"offset":    history.Offset,
	}
	return resultFromStructured(structured)
}

func resultFromStructured(structured map[string]any) (Result, error) {
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func toMissionEntry(mission missiondomain.Mission) missionEntry {
	return missionEntry{
		ID:          string(mission.ID),
		ProjectID:   mission.ProjectID,
		Title:       mission.Title,
		Description: mission.Description,
		Status:      string(mission.Status),
		StartDate:   formatOptionalTime(mission.StartDate),
		EndDate:     formatOptionalTime(mission.EndDate),
		CreatedAt:   formatTime(mission.CreatedAt),
		UpdatedAt:   formatTime(mission.UpdatedAt),
		PausedAt:    formatOptionalTime(mission.PausedAt),
		CompletedAt: formatOptionalTime(mission.CompletedAt),
	}
}

func toLeanMissionEntry(mission missiondomain.Mission) leanMissionEntry {
	return leanMissionEntry{
		ID:     string(mission.ID),
		Title:  mission.Title,
		Status: string(mission.Status),
	}
}

func toLeanKeyResultEntry(keyResult krdomain.KeyResult) leanKeyResultEntry {
	return leanKeyResultEntry{
		ID:              string(keyResult.ID),
		MissionID:       string(keyResult.MissionID),
		Title:           keyResult.Title,
		Status:          string(keyResult.Status),
		ProgressPercent: keyResult.ProgressPercent,
		Version:         keyResult.Version,
	}
}

func toKeyResultEntry(keyResult krdomain.KeyResult) keyResultEntry {
	return keyResultEntry{
		ID:                    string(keyResult.ID),
		MissionID:             string(keyResult.MissionID),
		Title:                 keyResult.Title,
		Description:           keyResult.Description,
		MeasurementType:       string(keyResult.MeasurementType),
		Direction:             string(keyResult.Direction),
		Unit:                  keyResult.Unit,
		Baseline:              keyResult.Baseline,
		TargetValue:           keyResult.TargetValue,
		CurrentValue:          keyResult.CurrentValue,
		ProgressPercent:       keyResult.ProgressPercent,
		Status:                string(keyResult.Status),
		LastMilestoneNotified: keyResult.LastMilestoneNotified,
		Version:               keyResult.Version,
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("mission: encode structured response: %w", err)
	}
	return string(encoded), nil
}
