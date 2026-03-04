package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/timeutil"
	"github.com/tinoosan/agen8/pkg/types"
)

type notificationCall struct {
	method string
	params any
}

func eventTime(now func() time.Time, ev types.EventRecord) time.Time {
	if !ev.Timestamp.IsZero() {
		return ev.Timestamp
	}
	if now != nil {
		v := now()
		return timeutil.OrNow(&v)
	}
	return timeutil.OrNow(nil)
}

func trimType(ev types.EventRecord) string {
	return strings.TrimSpace(ev.Type)
}

func mapGet(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[key])
}

func parseBoolString(s string) (bool, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "1", "t", "yes", "y":
		return true, true
	case "false", "0", "f", "no", "n":
		return false, true
	default:
		return false, false
	}
}

func parseIntString(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func newID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return uuid.NewString()
	}
	return prefix + uuid.NewString()
}

func ensureThreadID(threadID ThreadID, runID string) ThreadID {
	if strings.TrimSpace(string(threadID)) != "" {
		return threadID
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ThreadID("thread-" + uuid.NewString())
	}
	return ThreadID("thread-" + runID)
}

func turnStatusFromTaskStatus(taskStatus string) (TurnStatus, bool) {
	turnStatus, ok := taskStatusToTurnStatus[strings.ToLower(strings.TrimSpace(taskStatus))]
	return turnStatus, ok
}

var taskStatusToTurnStatus = map[string]TurnStatus{
	"pending":   TurnStatusPending,
	"active":    TurnStatusInProgress,
	"succeeded": TurnStatusCompleted,
	"success":   TurnStatusCompleted,
	"completed": TurnStatusCompleted,
	"failed":    TurnStatusFailed,
	"failure":   TurnStatusFailed,
	"error":     TurnStatusFailed,
	"canceled":  TurnStatusCanceled,
	"cancelled": TurnStatusCanceled,
}

func itemIDForTurn(turnID TurnID, suffix string) ItemID {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return ItemID(newID("item-"))
	}
	return ItemID(fmt.Sprintf("%s-%s", string(turnID), suffix))
}

func shouldSuppressOp(op, path string) bool {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	switch op {
	case "fs_list", "fs_stat", "fs_read":
		// Suppress high-frequency system scans that are not user-facing routing activity.
		return strings.HasPrefix(path, "/workspace/deliverables/") || strings.HasPrefix(path, "/workspace/quarantine/")
	default:
		return false
	}
}
