package app

import (
	"strings"

	"github.com/tinoosan/agen8/pkg/cost"
	"github.com/tinoosan/agen8/pkg/types"
)

const (
	defaultReasoningEffort  = "medium"
	defaultReasoningSummary = "auto"
)

func modelReasoningKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func normalizeReasoningSummaryValue(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "none" {
		return "off"
	}
	return v
}

func sessionReasoningForModel(sess types.Session, model, fallbackEffort, fallbackSummary string) (string, string) {
	model = strings.TrimSpace(model)
	effort := strings.TrimSpace(sess.ReasoningEffort)
	summary := normalizeReasoningSummaryValue(sess.ReasoningSummary)
	if key := modelReasoningKey(model); key != "" && len(sess.ReasoningByModel) != 0 {
		if pref, ok := sess.ReasoningByModel[key]; ok {
			if v := strings.TrimSpace(pref.Effort); v != "" {
				effort = v
			}
			if v := normalizeReasoningSummaryValue(pref.Summary); v != "" {
				summary = v
			}
		}
	}
	if effort == "" {
		effort = strings.TrimSpace(fallbackEffort)
	}
	if summary == "" {
		summary = normalizeReasoningSummaryValue(fallbackSummary)
	}
	if cost.SupportsReasoningSummary(model) {
		if effort == "" {
			effort = defaultReasoningEffort
		}
		if summary == "" {
			summary = defaultReasoningSummary
		}
	}
	return effort, summary
}

func ensureSessionReasoningForModel(sess *types.Session, model, fallbackEffort, fallbackSummary string) {
	if sess == nil {
		return
	}
	effort, summary := sessionReasoningForModel(*sess, model, fallbackEffort, fallbackSummary)
	sess.ReasoningEffort = effort
	sess.ReasoningSummary = summary
	if !cost.SupportsReasoningSummary(model) {
		return
	}
	key := modelReasoningKey(model)
	if key == "" {
		return
	}
	if sess.ReasoningByModel == nil {
		sess.ReasoningByModel = map[string]types.SessionReasoningConfig{}
	}
	sess.ReasoningByModel[key] = types.SessionReasoningConfig{Effort: effort, Summary: summary}
}

func storeSessionReasoningPreference(sess *types.Session, model, effort, summary string) {
	if sess == nil {
		return
	}
	effort = strings.TrimSpace(effort)
	summary = normalizeReasoningSummaryValue(summary)
	if effort != "" {
		sess.ReasoningEffort = effort
	}
	if summary != "" {
		sess.ReasoningSummary = summary
	}
	key := modelReasoningKey(model)
	if key == "" {
		return
	}
	if sess.ReasoningByModel == nil {
		sess.ReasoningByModel = map[string]types.SessionReasoningConfig{}
	}
	pref := sess.ReasoningByModel[key]
	if effort != "" {
		pref.Effort = effort
	}
	if summary != "" {
		pref.Summary = summary
	}
	sess.ReasoningByModel[key] = pref
}
