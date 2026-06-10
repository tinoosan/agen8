package decision

import (
	"encoding/json"
	"fmt"
	"strings"

	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
)

// decisionEntry is the model-facing response for decision.log. It carries only
// the new decision id — everything else the previous shape echoed (title,
// invalidationConditions, the refs, member id/name) was input the model just
// supplied, or a constant (kind="log", sourceType="agent"). The decision is
// still recorded in full server-side; the web reads its own DecisionView.
type decisionEntry struct {
	ID string `json:"id"`
}

func resultEntry(result decisionapp.Result) decisionEntry {
	return decisionEntry{
		ID: strings.TrimSpace(result.ID),
	}
}

func resultFromStructured(structured map[string]any) (Result, error) {
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("decision: encode structured response: %w", err)
	}
	return string(encoded), nil
}
