package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

const defaultLifecycleHistoryLimit = 50
const maxLifecycleHistoryLimit = 100

func (s *Service) GetLifecycleHistory(ctx context.Context, missionID missiondomain.MissionID, filter LifecycleHistoryFilter) (LifecycleHistory, error) {
	missionID = missiondomain.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return LifecycleHistory{}, fmt.Errorf("mission id is required")
	}
	if _, err := s.missions.GetMission(ctx, missionID); err != nil {
		return LifecycleHistory{}, err
	}
	if filter.Limit < 0 {
		return LifecycleHistory{}, fmt.Errorf("lifecycle history limit must be non-negative")
	}
	if filter.Offset < 0 {
		return LifecycleHistory{}, fmt.Errorf("lifecycle history offset must be non-negative")
	}
	if filter.Limit == 0 {
		filter.Limit = defaultLifecycleHistoryLimit
	}
	if filter.Limit > maxLifecycleHistoryLimit {
		filter.Limit = maxLifecycleHistoryLimit
	}
	if len(filter.Types) == 0 {
		filter.Types = lifecycleHistoryEventTypes()
	}
	events, count, err := s.lifecycleEvents.ListLifecycleEvents(ctx, missionID, filter)
	if err != nil {
		return LifecycleHistory{}, err
	}
	entries := make([]LifecycleHistoryEntry, 0, len(events))
	for _, event := range events {
		entryMissionID := missionID
		if raw := strings.TrimSpace(event.Data["missionId"]); raw != "" {
			entryMissionID = missiondomain.MissionID(raw)
		}
		entries = append(entries, LifecycleHistoryEntry{
			EventID:         string(event.EventID),
			MissionID:       entryMissionID,
			KeyResultID:     keyResultIDFromData(event.Data),
			Type:            event.Type,
			Action:          lifecycleAction(event.Type),
			Status:          event.Data["status"],
			Note:            event.Data["note"],
			Actor:           event.Data["actor"],
			Origin:          event.Data["origin"],
			Message:         event.Message,
			ProgressPercent: event.Data["progressPercent"],
			Timestamp:       parseEventTimestamp(event.CreatedAt),
		})
	}
	return LifecycleHistory{
		MissionID: missionID,
		Entries:   entries,
		Count:     count,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}, nil
}

func lifecycleHistoryEventTypes() []string {
	return []string{
		string(MissionEventActivated),
		string(MissionEventPaused),
		string(MissionEventCompleted),
		string(MissionEventArchived),
		string(KREventCreated),
		string(KREventCompleted),
		string(KREventDropped),
		string(KREventReopened),
		string(KREventOwnerAssigned),
	}
}

func isLifecycleHistoryEvent(kind MissionEventKind) bool {
	for _, eventType := range lifecycleHistoryEventTypes() {
		if eventType == string(kind) {
			return true
		}
	}
	return false
}

func lifecycleAction(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case string(MissionEventActivated):
		return "activate"
	case string(MissionEventPaused):
		return "pause"
	case string(MissionEventCompleted):
		return "complete"
	case string(MissionEventArchived):
		return "archive"
	case string(KREventCreated):
		return "kr_create"
	case string(KREventCompleted):
		return "kr_complete"
	case string(KREventDropped):
		return "kr_drop"
	case string(KREventReopened):
		return "kr_reopen"
	case string(KREventOwnerAssigned):
		return "kr_assign_space"
	default:
		return eventType
	}
}

func keyResultIDFromData(data map[string]string) krdomain.KeyResultID {
	if data == nil {
		return ""
	}
	return krdomain.KeyResultID(strings.TrimSpace(data["keyResultId"]))
}

func parseEventTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
