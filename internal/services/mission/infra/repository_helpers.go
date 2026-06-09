package infra

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

func missionWhere(filter mission.MissionFilter) (string, []any) {
	var clauses []string
	var args []any
	if strings.TrimSpace(filter.ProjectID) != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			status = mission.MissionStatus(strings.TrimSpace(string(status)))
			if status == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, string(status))
		}
		if len(placeholders) > 0 {
			clauses = append(clauses, "status IN ("+strings.Join(placeholders, ", ")+")")
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func unmarshalMission(raw []byte) (mission.Mission, error) {
	if len(raw) == 0 {
		return mission.Mission{}, fmt.Errorf("mission_json is empty")
	}
	var out mission.Mission
	if err := json.Unmarshal(raw, &out); err != nil {
		return mission.Mission{}, err
	}
	return out, nil
}

func unmarshalKeyResult(raw []byte) (kr.KeyResult, error) {
	if len(raw) == 0 {
		return kr.KeyResult{}, fmt.Errorf("key_result_json is empty")
	}
	var out kr.KeyResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return kr.KeyResult{}, err
	}
	return out, nil
}

func optionalTimeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
