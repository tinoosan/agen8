package adapter

import (
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/opmeta"
	"github.com/tinoosan/agen8/pkg/types"
)

// EventRecordToActivity maps an incoming EventRecord to an Activity for TUI streams.
// Returns (Activity, true) if it can be represented incrementally (e.g., a new operation request).
// Returns (Activity, false) if the event requires a full fetch (e.g., an operation response or unknown event type).
func EventRecordToActivity(ev types.EventRecord) (types.Activity, bool) {
	typ := strings.TrimSpace(ev.Type)

	switch typ {
	case "agent.op.request":
		op := ""
		eventData := ev.Data
		if ev.Data != nil {
			op = strings.TrimSpace(ev.Data["op"])
			if strings.EqualFold(op, "tool_result") {
				if tag := strings.ToLower(strings.TrimSpace(ev.Data["tag"])); tag == "task_create" || tag == "task_review" || tag == "obsidian" || tag == "soul_update" {
					op = tag
				}
			}
		}
		if eventData != nil {
			copied := make(map[string]string, len(eventData)+1)
			for k, v := range eventData {
				copied[k] = v
			}
			if strings.TrimSpace(op) != "" {
				copied["op"] = op
			}
			eventData = copied
		}
		if op == "" {
			return types.Activity{}, false
		}
		if opmeta.ShouldHideRoutingNoiseOp(op, strings.TrimSpace(ev.Data["path"])) {
			return types.Activity{}, false
		}

		actID := ""
		if ev.Data != nil {
			actID = strings.TrimSpace(ev.Data["opId"])
		}
		if actID == "" {
			actID = ev.EventID
		}
		// Prefix with runID to match server formatting
		if runID := strings.TrimSpace(ev.RunID); runID != "" && actID != "" && !strings.HasPrefix(actID, runID+"|") {
			actID = runID + "|" + actID
		}

		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}

		act := types.Activity{
			ID:            actID,
			Kind:          op,
			Title:         opmeta.FormatRequestTitle(eventData),
			Status:        types.ActivityPending,
			StartedAt:     ts,
			Path:          strings.TrimSpace(ev.Data["path"]),
			MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
			TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
			TextTruncated: parseBool(ev.Data["textTruncated"]),
			TextRedacted:  parseBool(ev.Data["textRedacted"]),
			TextIsJSON:    parseBool(ev.Data["textIsJSON"]),
			TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
			Data:          eventData,
		}
		return act, true

	case "task.done":
		summary := ""
		taskID := ""
		if ev.Data != nil {
			summary = strings.TrimSpace(ev.Data["summary"])
			taskID = strings.TrimSpace(ev.Data["taskId"])
		}
		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		actID := taskID
		if actID == "" {
			actID = ev.EventID
		}
		act := types.Activity{
			ID:        actID,
			Kind:      typ,
			Status:    types.ActivityOK,
			StartedAt: ts,
			Data:      ev.Data,
		}
		if summary != "" {
			act.Title = summary
			act.OutputPreview = summary
		}
		if fin := ts; !fin.IsZero() {
			act.FinishedAt = &fin
		}
		return act, true

	case "user_message", "agent_speak", "model_response", "task.create":
		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		act := types.Activity{
			ID:        ev.EventID,
			Kind:      typ,
			Title:     strings.TrimSpace(ev.Message),
			Status:    types.ActivityOK,
			StartedAt: ts,
			Data:      ev.Data,
		}
		if fin := ts; !fin.IsZero() {
			act.FinishedAt = &fin
		}
		return act, true

	default:
		// agent.op.response, model.thinking.*, etc.
		// Trigger a fallback full fetch so the state is reconciled properly.
		return types.Activity{}, false
	}
}

func parseBool(s string) bool {
	return strings.TrimSpace(s) == "true"
}
