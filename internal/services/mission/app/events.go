package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type MissionEventKind string

const (
	MissionEventCreated   MissionEventKind = "mission.created"
	MissionEventActivated MissionEventKind = "mission.activated"
	MissionEventPaused    MissionEventKind = "mission.paused"
	MissionEventCompleted MissionEventKind = "mission.completed"
	MissionEventArchived  MissionEventKind = "mission.archived"
	// MissionEventPurged announces a permanent hard delete. It is intentionally
	// NOT a lifecycle-history event (see lifecycleHistoryEventTypes): the mission
	// and its rows are gone, so persisting a history row would only orphan it.
	// This event exists purely to notify live SSE subscribers.
	MissionEventPurged MissionEventKind = "mission.purged"

	KREventProgressUpdated MissionEventKind = "kr.progress_updated"
	KREventMilestone       MissionEventKind = "kr.milestone"
	KREventCompleted       MissionEventKind = "kr.completed"
	KREventCreated         MissionEventKind = "kr.created"
	KREventDropped         MissionEventKind = "kr.dropped"
	KREventReopened        MissionEventKind = "kr.reopened"
	KREventOwnerAssigned   MissionEventKind = "kr.owner_assigned"
)

type missionEventTemplate struct {
	EventType string
	Message   string
}

func missionEventSpec(kind MissionEventKind) (missionEventTemplate, error) {
	return missionEventTemplate{EventType: string(kind), Message: string(kind)}, nil
}

func (s *Service) publishMissionEvent(ctx context.Context, kind MissionEventKind, mission missiondomain.Mission) error {
	return s.publishMissionEventWithData(ctx, kind, mission, nil)
}

func (s *Service) publishMissionEventWithData(ctx context.Context, kind MissionEventKind, mission missiondomain.Mission, extra map[string]string) error {
	spec, err := missionEventSpec(kind)
	if err != nil {
		return err
	}
	event := types.EventRecord{
		RunID:     types.RunID(mission.ID),
		CreatedAt: eventCreatedAt(s.clock.Now()),
		Type:      spec.EventType,
		Message:   spec.Message,
		Data: map[string]string{
			"origin":    "mission",
			"missionId": string(mission.ID),
			"projectId": mission.ProjectID,
			"status":    string(mission.Status),
		},
	}
	for key, value := range extra {
		event.Data[key] = value
	}
	if err := s.recordLifecycleEvent(ctx, kind, event); err != nil {
		return err
	}
	if err := s.events.Append(ctx, event); err != nil {
		return fmt.Errorf("publish mission event %s: %w", kind, err)
	}
	return nil
}

func noteData(note string) map[string]string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	return map[string]string{"note": note}
}

func eventCreatedAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// keyResultProjectID resolves the owning project for a KR via its mission. The
// KeyResult aggregate carries no projectId of its own (that field was removed in
// ef824e74 "Drop dead KR owner project plumbing"), but realtime fan-out keys on
// EventRecord.Data["projectId"] (internal/daemon/events_hub.go), so KR events
// must carry it or they get dropped. Best-effort: a lookup miss returns "" and
// the caller omits the field rather than failing the originating operation.
func (s *Service) keyResultProjectID(ctx context.Context, keyResult krdomain.KeyResult) string {
	mission, err := s.missions.GetMission(ctx, keyResult.MissionID)
	if err != nil {
		return ""
	}
	return mission.ProjectID
}

func (s *Service) publishKREvent(ctx context.Context, kind MissionEventKind, keyResult krdomain.KeyResult) error {
	return s.publishKREventWithData(ctx, kind, keyResult, nil)
}

func (s *Service) publishKRMilestoneEvent(ctx context.Context, keyResult krdomain.KeyResult, milestone int) error {
	return s.publishKREventWithData(ctx, KREventMilestone, keyResult, map[string]string{
		"milestone": strconv.Itoa(milestone),
	})
}

func (s *Service) publishKREventWithData(ctx context.Context, kind MissionEventKind, keyResult krdomain.KeyResult, extra map[string]string) error {
	spec, err := missionEventSpec(kind)
	if err != nil {
		return err
	}
	event := types.EventRecord{
		RunID:     types.RunID(keyResult.MissionID),
		CreatedAt: eventCreatedAt(s.clock.Now()),
		Type:      spec.EventType,
		Message:   spec.Message,
		Data: map[string]string{
			"origin":          "mission",
			"missionId":       string(keyResult.MissionID),
			"keyResultId":     string(keyResult.ID),
			"status":          string(keyResult.Status),
			"measurementType": string(keyResult.MeasurementType),
			"progressPercent": strconv.Itoa(keyResult.ProgressPercent),
		},
	}
	// Realtime fan-out drops events whose record has no projectId. The KR
	// aggregate doesn't carry one, so resolve it from the mission.
	if projectID := s.keyResultProjectID(ctx, keyResult); projectID != "" {
		event.Data["projectId"] = projectID
	}
	for key, value := range extra {
		event.Data[key] = value
	}
	if err := s.recordLifecycleEvent(ctx, kind, event); err != nil {
		return err
	}
	if err := s.events.Append(ctx, event); err != nil {
		return fmt.Errorf("publish KR event %s: %w", kind, err)
	}
	return nil
}

func (s *Service) recordLifecycleEvent(ctx context.Context, kind MissionEventKind, event types.EventRecord) error {
	if !isLifecycleHistoryEvent(kind) {
		return nil
	}
	if err := s.lifecycleEvents.AppendLifecycleEvent(ctx, event); err != nil {
		return fmt.Errorf("record lifecycle event %s: %w", kind, err)
	}
	return nil
}

func (s *Service) recordMissionLifecycleNote(ctx context.Context, kind MissionEventKind, mission missiondomain.Mission, note string) error {
	extra := noteData(note)
	if extra == nil {
		return nil
	}
	extra["idempotent"] = "true"
	spec, err := missionEventSpec(kind)
	if err != nil {
		return err
	}
	event := types.EventRecord{
		RunID:     types.RunID(mission.ID),
		CreatedAt: eventCreatedAt(s.clock.Now()),
		Type:      spec.EventType,
		Message:   spec.Message,
		Data: map[string]string{
			"origin":    "mission",
			"missionId": string(mission.ID),
			"projectId": mission.ProjectID,
			"status":    string(mission.Status),
		},
	}
	for key, value := range extra {
		event.Data[key] = value
	}
	return s.recordLifecycleEvent(ctx, kind, event)
}

func (s *Service) recordKRLifecycleNote(ctx context.Context, kind MissionEventKind, keyResult krdomain.KeyResult, note string) error {
	extra := noteData(note)
	if extra == nil {
		return nil
	}
	extra["idempotent"] = "true"
	spec, err := missionEventSpec(kind)
	if err != nil {
		return err
	}
	event := types.EventRecord{
		RunID:     types.RunID(keyResult.MissionID),
		CreatedAt: eventCreatedAt(s.clock.Now()),
		Type:      spec.EventType,
		Message:   spec.Message,
		Data: map[string]string{
			"origin":          "mission",
			"missionId":       string(keyResult.MissionID),
			"keyResultId":     string(keyResult.ID),
			"status":          string(keyResult.Status),
			"measurementType": string(keyResult.MeasurementType),
			"progressPercent": strconv.Itoa(keyResult.ProgressPercent),
		},
	}
	if projectID := s.keyResultProjectID(ctx, keyResult); projectID != "" {
		event.Data["projectId"] = projectID
	}
	for key, value := range extra {
		event.Data[key] = value
	}
	return s.recordLifecycleEvent(ctx, kind, event)
}
