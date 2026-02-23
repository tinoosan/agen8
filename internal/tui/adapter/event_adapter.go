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
		if ev.Data != nil {
			op = strings.TrimSpace(ev.Data["op"])
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
			Title:         opmeta.FormatRequestTitle(ev.Data),
			Status:        types.ActivityPending,
			StartedAt:     ts,
			Path:          strings.TrimSpace(ev.Data["path"]),
			MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
			TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
			TextTruncated: parseBool(ev.Data["textTruncated"]),
			TextRedacted:  parseBool(ev.Data["textRedacted"]),
			TextIsJSON:    parseBool(ev.Data["textIsJSON"]),
			TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
			Data:          ev.Data,
		}
		return act, true

	case "user_message", "task.done", "agent_speak", "model_response", "task.create":
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
